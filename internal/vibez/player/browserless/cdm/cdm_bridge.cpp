#include "cdm_bridge.h"
#include "content_decryption_module.h"

#include <iostream>
#include <atomic>
#include <algorithm>
#include <vector>
#include <chrono>
#include <cstring>
#include <mutex>
#include <condition_variable>
#include <cstdlib>

#ifdef _WIN32
#include <windows.h>
#else
#include <dlfcn.h>
#include <sys/time.h>
#endif

namespace {

// Portable shared-library and wall-clock helpers. The CDM is a plain C-ABI
// shared object everywhere; only how we load it differs.
#ifdef _WIN32
void* load_library(const char* utf8_path) {
    int n = MultiByteToWideChar(CP_UTF8, 0, utf8_path, -1, nullptr, 0);
    if (n <= 0) return nullptr;
    std::vector<wchar_t> wide(static_cast<size_t>(n));
    MultiByteToWideChar(CP_UTF8, 0, utf8_path, -1, wide.data(), n);
    // LOAD_WITH_ALTERED_SEARCH_PATH resolves the DLL's own dependencies from its directory.
    return reinterpret_cast<void*>(LoadLibraryExW(wide.data(), nullptr, LOAD_WITH_ALTERED_SEARCH_PATH));
}
void* load_symbol(void* lib, const char* name) {
    return reinterpret_cast<void*>(GetProcAddress(static_cast<HMODULE>(lib), name));
}
void unload_library(void* lib) { FreeLibrary(static_cast<HMODULE>(lib)); }
double wall_time_seconds() {
    FILETIME ft;
    GetSystemTimeAsFileTime(&ft);
    ULARGE_INTEGER u;
    u.LowPart = ft.dwLowDateTime;
    u.HighPart = ft.dwHighDateTime;
    // 100ns ticks since 1601-01-01 -> seconds since the Unix epoch.
    return static_cast<double>(u.QuadPart - 116444736000000000ULL) / 10000000.0;
}
#else
void* load_library(const char* path) { return dlopen(path, RTLD_NOW); }
void* load_symbol(void* lib, const char* name) { return dlsym(lib, name); }
void unload_library(void* lib) { dlclose(lib); }
double wall_time_seconds() {
    struct timeval tv;
    gettimeofday(&tv, nullptr);
    return static_cast<double>(tv.tv_sec) + static_cast<double>(tv.tv_usec) / 1000000.0;
}
#endif

class BufferImpl : public cdm::Buffer {
    std::vector<uint8_t> data_;
public:
    BufferImpl(uint32_t cap) : data_(cap) {}
    void Destroy() override { delete this; }
    uint32_t Capacity() const override { return static_cast<uint32_t>(data_.capacity()); }
    uint8_t* Data() override { return data_.data(); }
    void SetSize(uint32_t s) override { data_.resize(s); }
    uint32_t Size() const override { return static_cast<uint32_t>(data_.size()); }
};

class DecryptedBlockImpl : public cdm::DecryptedBlock {
    cdm::Buffer* buffer_ = nullptr;
    int64_t ts_ = 0;
public:
    void SetDecryptedBuffer(cdm::Buffer* b) override {
        if (buffer_) buffer_->Destroy();
        buffer_ = b;
    }
    cdm::Buffer* DecryptedBuffer() override { return buffer_; }
    void SetTimestamp(int64_t ts) override { ts_ = ts; }
    int64_t Timestamp() const override { return ts_; }
    ~DecryptedBlockImpl() {
        if (buffer_) buffer_->Destroy();
    }
};

class VideoFrameImpl : public cdm::VideoFrame {
   public:
    cdm::VideoFormat format = cdm::kUnknownVideoFormat;
    cdm::Size size{};
    cdm::Buffer* buffer = nullptr;
    uint32_t offsets[3]{}, strides[3]{};
    int64_t timestamp = 0;
    ~VideoFrameImpl() {
        if (buffer) buffer->Destroy();
    }
    void SetFormat(cdm::VideoFormat f) override { format = f; }
    cdm::VideoFormat Format() const override { return format; }
    void SetSize(cdm::Size s) override { size = s; }
    cdm::Size Size() const override { return size; }
    void SetFrameBuffer(cdm::Buffer* b) override {
        if (buffer) buffer->Destroy();
        buffer = b;
    }
    cdm::Buffer* FrameBuffer() override { return buffer; }
    void SetPlaneOffset(cdm::VideoPlane p, uint32_t n) override {
        if (p < 3) offsets[p] = n;
    }
    uint32_t PlaneOffset(cdm::VideoPlane p) override { return p < 3 ? offsets[p] : 0; }
    void SetStride(cdm::VideoPlane p, uint32_t n) override {
        if (p < 3) strides[p] = n;
    }
    uint32_t Stride(cdm::VideoPlane p) override { return p < 3 ? strides[p] : 0; }
    void SetTimestamp(int64_t t) override { timestamp = t; }
    int64_t Timestamp() const override { return timestamp; }
};

class Host11Impl : public cdm::Host_11 {
   public:
    std::mutex cv_m;
    std::condition_variable msg_cv;
    std::condition_variable update_cv;
    std::condition_variable cert_cv;

    bool initialized = false;
    std::string sessionId;
    std::vector<uint8_t> sessionMessage;
    bool hasMessage = false;
    bool updateResolved = false;
    bool updateFailed = false;
    bool certResolved = false;
    bool certFailed = false;
    int usableKeys = 0;
    std::atomic<bool> outputQueryPending{false};
    struct Timer {
        std::chrono::steady_clock::time_point at;
        void* context;
    };
    std::vector<Timer> timers;

    cdm::Buffer* Allocate(uint32_t capacity) override { return new BufferImpl(capacity); }

    void SetTimer(int64_t delay_ms, void* context) override {
        std::lock_guard<std::mutex> lock(cv_m);
        if (timers.size() < 128)
            timers.push_back({std::chrono::steady_clock::now() +
                                  std::chrono::milliseconds(std::max<int64_t>(0, delay_ms)),
                              context});
    }

    cdm::Time GetCurrentWallTime() override { return static_cast<cdm::Time>(wall_time_seconds()); }

    void OnInitialized(bool success) override {
        std::lock_guard<std::mutex> lk(cv_m);
        initialized = success;
    }

    void OnResolveKeyStatusPromise(uint32_t promise_id, cdm::KeyStatus key_status) override {}

    void OnResolveNewSessionPromise(uint32_t promise_id, const char* session_id, uint32_t session_id_size) override {
        std::lock_guard<std::mutex> lk(cv_m);
        sessionId = std::string(session_id, session_id_size);
        msg_cv.notify_all();
    }

    void OnResolvePromise(uint32_t promise_id) override {
        std::lock_guard<std::mutex> lk(cv_m);
        if (promise_id == 1) {
            certResolved = true;
            cert_cv.notify_all();
        } else if (promise_id == 3) {
            updateResolved = true;
            update_cv.notify_all();
        }
    }

    void OnRejectPromise(uint32_t promise_id, cdm::Exception exception, uint32_t system_code, const char* error_message, uint32_t error_message_size) override {
        std::lock_guard<std::mutex> lk(cv_m);
        if (promise_id == 1) {
            certFailed = true;
            cert_cv.notify_all();
        } else if (promise_id == 3) {
            updateFailed = true;
            update_cv.notify_all();
        }
    }

    void OnSessionMessage(const char* session_id, uint32_t session_id_size, cdm::MessageType message_type, const char* message, uint32_t message_size) override {
        std::lock_guard<std::mutex> lk(cv_m);
        sessionMessage.assign(message, message + message_size);
        hasMessage = true;
        msg_cv.notify_all();
    }

    void OnSessionKeysChange(const char* session_id, uint32_t session_id_size, bool has_additional_usable_key, const cdm::KeyInformation* keys_info, uint32_t keys_info_count) override {
        std::lock_guard<std::mutex> lk(cv_m);
        for (uint32_t i = 0; i < keys_info_count; i++) {
            if (keys_info[i].status == cdm::kUsable) {
                usableKeys++;
            }
        }
    }

    void OnExpirationChange(const char* session_id, uint32_t session_id_size, cdm::Time new_expiry_time) override {}

    void OnSessionClosed(const char* session_id, uint32_t session_id_size) override {}

    void SendPlatformChallenge(const char* service_id, uint32_t service_id_size, const char* challenge, uint32_t challenge_size) override {}

    void EnableOutputProtection(uint32_t desired_protection_mask) override {}

    void QueryOutputProtectionStatus() override { outputQueryPending = true; }

    void OnDeferredInitializationDone(cdm::StreamType stream_type, cdm::Status decoder_status) override {}

    cdm::FileIO* CreateFileIO(cdm::FileIOClient* client) override {
        return nullptr;
    }

    void RequestStorageId(uint32_t version) override {}

    void ReportMetrics(cdm::MetricName metric_name, uint64_t value) override {}
};

void* GetCdmHostCallback(int host_interface_version, void* user_data) {
    if (host_interface_version == 11) {
        return static_cast<cdm::Host_11*>(user_data);
    }
    return nullptr;
}

} // namespace

struct cdm_context {
    void* lib_handle = nullptr;
    cdm::ContentDecryptionModule_11* cdm11 = nullptr;
    Host11Impl host;
};

// Go serializes entry points. Pump callbacks on that same CDM execution path,
// without detached threads that could outlive the CDM or race a decryption.
static void pump_host(cdm_context_t* ctx) {
    if (ctx->host.outputQueryPending.exchange(false))
        ctx->cdm11->OnQueryOutputProtectionStatus(cdm::kQuerySucceeded, cdm::kLinkTypeUnknown,
                                                  cdm::kProtectionNone);
    for (int n = 0; n < 32; n++) {
        void* context = nullptr;
        bool found = false;
        {
            std::lock_guard<std::mutex> lock(ctx->host.cv_m);
            auto now = std::chrono::steady_clock::now();
            for (auto it = ctx->host.timers.begin(); it != ctx->host.timers.end(); ++it) {
                if (it->at <= now) {
                    context = it->context;
                    ctx->host.timers.erase(it);
                    found = true;
                    break;
                }
            }
        }
        if (!found) break;
        ctx->cdm11->TimerExpired(context);
    }
}

extern "C" {

cdm_context_t* cdm_context_create(const char* library_path) {
    if (!library_path) return nullptr;

    void* handle = load_library(library_path);
    if (!handle) {
        return nullptr;
    }

    using InitFunc = void (*)(void);
    using CreateFunc = void* (*)(int, const char*, uint32_t, GetCdmHostFunc, void*);

    InitFunc init_cdm = reinterpret_cast<InitFunc>(load_symbol(handle, "InitializeCdmModule_4"));
    CreateFunc create_cdm = reinterpret_cast<CreateFunc>(load_symbol(handle, "CreateCdmInstance"));

    if (!init_cdm) {
        init_cdm = reinterpret_cast<InitFunc>(load_symbol(handle, "InitializeCdmModule"));
    }

    if (!create_cdm) {
        unload_library(handle);
        return nullptr;
    }

    if (init_cdm) {
        init_cdm();
    }

    cdm_context_t* ctx = new cdm_context();
    ctx->lib_handle = handle;

    std::string key_system = "com.widevine.alpha";
    void* raw = create_cdm(11, key_system.data(), static_cast<uint32_t>(key_system.size()), GetCdmHostCallback, &ctx->host);
    if (!raw) {
        delete ctx;
        unload_library(handle);
        return nullptr;
    }

    ctx->cdm11 = static_cast<cdm::ContentDecryptionModule_11*>(raw);
    ctx->cdm11->Initialize(true, false, false);
    pump_host(ctx);

    return ctx;
}

int cdm_context_set_server_certificate(cdm_context_t* ctx, const uint8_t* cert_data, uint32_t cert_size) {
    if (!ctx || !ctx->cdm11) return -1;
    {
        std::lock_guard<std::mutex> lk(ctx->host.cv_m);
        ctx->host.certResolved = false;
        ctx->host.certFailed = false;
    }
    ctx->cdm11->SetServerCertificate(1, cert_data, cert_size);
    {
        std::unique_lock<std::mutex> lk(ctx->host.cv_m);
        bool ok = ctx->host.cert_cv.wait_for(lk, std::chrono::seconds(5), [&]{
            return ctx->host.certResolved || ctx->host.certFailed;
        });
        if (!ok || ctx->host.certFailed) {
            return -2;
        }
    }
    return 0;
}

int cdm_context_generate_challenge(
    cdm_context_t* ctx,
    const uint8_t* kid,
    uint32_t kid_size,
    uint8_t** out_challenge,
    uint32_t* out_challenge_size,
    char** out_session_id
) {
    if (!ctx || !ctx->cdm11 || !kid || (kid_size != 16 && (kid_size < 32 || kid_size > 65536 || std::memcmp(kid + 4, "pssh", 4) != 0))) return -1;

    // Reset host session state
    {
        std::lock_guard<std::mutex> lk(ctx->host.cv_m);
        ctx->host.sessionId.clear();
        ctx->host.sessionMessage.clear();
        ctx->host.hasMessage = false;
    }

    // Widevine PSSH header box (52 bytes) matching MusicKit v3 generatePSSH:
    //   [0..3]   size = 52 (32-byte box header + 4-byte protobuf prefix + 16-byte KID)
    //   [4..7]   "pssh"
    //   [8..11]  version = 0, flags = 0
    //   [12..27] Widevine SystemID: edef8ba9-79d6-4ace-a3c8-27dcd51d21ed
    //   [28..31] data_size = 20 (4 + 16)
    //   [32..35] WidevinePsshData protobuf: 0x08 0x01 (algorithm=AESCTR) 0x12 0x10 (key_id, len=16)
    //   [36..51] 16-byte Key ID
    std::vector<uint8_t> pssh = {
        0x00, 0x00, 0x00, 0x34,
        'p',  's',  's',  'h',
        0x00, 0x00, 0x00, 0x00,
        0xed, 0xef, 0x8b, 0xa9, 0x79, 0xd6, 0x4a, 0xce,
        0xa3, 0xc8, 0x27, 0xdc, 0xd5, 0x1d, 0x21, 0xed,
        0x00, 0x00, 0x00, 0x14,
        0x08, 0x01, 0x12, 0x10
    };
    if (kid_size == 16) pssh.insert(pssh.end(), kid, kid + kid_size);
    else pssh.assign(kid, kid + kid_size);

    ctx->cdm11->CreateSessionAndGenerateRequest(
        2,
        cdm::SessionType::kTemporary,
        cdm::InitDataType::kCenc,
        pssh.data(),
        static_cast<uint32_t>(pssh.size())
    );

    {
        std::unique_lock<std::mutex> lk(ctx->host.cv_m);
        bool ok = ctx->host.msg_cv.wait_for(lk, std::chrono::seconds(5), [&]{
            return ctx->host.hasMessage && !ctx->host.sessionId.empty();
        });
        if (!ok) {
            return -2;
        }
    }

    {
        std::lock_guard<std::mutex> lk(ctx->host.cv_m);
        *out_challenge_size = static_cast<uint32_t>(ctx->host.sessionMessage.size());
        *out_challenge = static_cast<uint8_t*>(std::malloc(*out_challenge_size));
        if (!*out_challenge) {
            return -3;
        }
        std::memcpy(*out_challenge, ctx->host.sessionMessage.data(), *out_challenge_size);

        *out_session_id = static_cast<char*>(std::malloc(ctx->host.sessionId.size() + 1));
        if (*out_session_id) std::memcpy(*out_session_id, ctx->host.sessionId.c_str(), ctx->host.sessionId.size() + 1);
        if (!*out_session_id) {
            std::free(*out_challenge);
            *out_challenge = nullptr;
            return -4;
        }
    }

    return 0;
}

int cdm_context_update_session(
    cdm_context_t* ctx,
    const char* session_id,
    const uint8_t* license_data,
    uint32_t license_size
) {
    if (!ctx || !ctx->cdm11 || !session_id || !license_data) return -1;

    {
        std::lock_guard<std::mutex> lk(ctx->host.cv_m);
        ctx->host.updateResolved = false;
        ctx->host.updateFailed = false;
        ctx->host.usableKeys = 0;
    }

    ctx->cdm11->UpdateSession(
        3,
        session_id,
        static_cast<uint32_t>(std::strlen(session_id)),
        license_data,
        license_size
    );

    {
        std::unique_lock<std::mutex> lk(ctx->host.cv_m);
        bool ok = ctx->host.update_cv.wait_for(lk, std::chrono::seconds(5), [&]{
            return ctx->host.updateResolved || ctx->host.updateFailed;
        });
        if (!ok || ctx->host.updateFailed) {
            return -2;
        }
        if (ctx->host.usableKeys == 0) {
            return -3;
        }
    }

    return 0;
}

int cdm_context_decrypt(
    cdm_context_t* ctx,
    const uint8_t* key_id,
    uint32_t key_id_size,
    const uint8_t* iv,
    uint32_t iv_size,
    const uint8_t* in_data,
    uint32_t in_size,
    const uint16_t* clear_bytes,
    const uint32_t* cipher_bytes,
    uint32_t num_subsamples,
    uint8_t* out_decrypted,
    uint32_t* out_size,
    int cbcs, uint32_t crypt_blocks, uint32_t skip_blocks
) {
    if (!ctx || !ctx->cdm11 || !in_data || in_size == 0) return -1;

    pump_host(ctx);
    cdm::InputBuffer_2 input = {};
    input.data = in_data;
    input.data_size = in_size;
    input.encryption_scheme = cbcs ? cdm::EncryptionScheme::kCbcs : cdm::EncryptionScheme::kCenc;
    input.pattern = {crypt_blocks, skip_blocks};
    input.key_id = key_id;
    input.key_id_size = key_id_size;
    input.iv = iv;
    input.iv_size = iv_size;

    cdm::SubsampleEntry default_subsample = {0, in_size};
    std::vector<cdm::SubsampleEntry> subs;
    if (num_subsamples > 0 && clear_bytes && cipher_bytes) {
        subs.resize(num_subsamples);
        for (uint32_t i = 0; i < num_subsamples; ++i) {
            subs[i].clear_bytes = clear_bytes[i];
            subs[i].cipher_bytes = cipher_bytes[i];
        }
        input.subsamples = subs.data();
        input.num_subsamples = num_subsamples;
    } else {
        input.subsamples = &default_subsample;
        input.num_subsamples = 1;
    }

    DecryptedBlockImpl output;
    cdm::Status status = ctx->cdm11->Decrypt(input, &output);
    if (status != cdm::kSuccess) {
        return static_cast<int>(status);
    }

    if (!output.DecryptedBuffer() || output.DecryptedBuffer()->Size() == 0) {
        return -10;
    }

    if (output.DecryptedBuffer()->Size() > in_size) {
        return -11;
    }

    *out_size = output.DecryptedBuffer()->Size();
    std::memcpy(out_decrypted, output.DecryptedBuffer()->Data(), *out_size);

    return 0;
}

void cdm_context_destroy(cdm_context_t* ctx) {
    if (!ctx) return;
    if (ctx->cdm11) {
        ctx->cdm11->Destroy();
        ctx->cdm11 = nullptr;
    }
    // We intentionally do not call unload_library here: Widevine CDM spins up
    // internal threads and TLS destructors that expect the library code to
    // remain mapped until process termination.
    ctx->lib_handle = nullptr;
    delete ctx;
}

}  // extern "C"
extern "C" int cdm_configure_video(cdm_context_t* ctx,uint8_t* extra,uint32_t length,int width,int height){
    if (!ctx || !ctx->cdm11 || width < 1 || height < 1 || width > 1920 || height > 1080) return -1;
    ctx->cdm11->DeinitializeDecoder(cdm::kStreamTypeVideo);
    cdm::VideoDecoderConfig_2 config{};
    config.codec = cdm::kCodecH264;
    if (length < 2) return -1;
    switch (extra[1]) {
        case 66:
            config.profile = cdm::kH264ProfileBaseline;
            break;
        case 77:
            config.profile = cdm::kH264ProfileMain;
            break;
        case 100:
            config.profile = cdm::kH264ProfileHigh;
            break;
        default:
            return -1;
    }
    config.format = cdm::kI420;
    config.coded_size = {(width + 15) & ~15, (height + 15) & ~15};
    config.extra_data = extra;
    config.extra_data_size = length;
    config.encryption_scheme = cdm::EncryptionScheme::kCbcs;
    return ctx->cdm11->InitializeVideoDecoder(config);
}
extern "C" void cdm_reset_video(cdm_context_t* ctx) {
    if (ctx && ctx->cdm11) ctx->cdm11->DeinitializeDecoder(cdm::kStreamTypeVideo);
}
extern "C" int cdm_decode_video(cdm_context_t* ctx, const uint8_t* key, const uint8_t* iv,
                                uint32_t iv_size, const uint8_t* data, uint32_t length,
                                const uint16_t* clear, const uint32_t* cipher, uint32_t count,
                                uint32_t crypt, uint32_t skip, int64_t timestamp, int64_t earliest,
                                uint8_t** rgb, int* width, int* height, int64_t* pts) {
    if (!ctx || !ctx->cdm11 || !data || !length) return -1;
    pump_host(ctx);
    cdm::InputBuffer_2 input{};
    input.data = data;
    input.data_size = length;
    input.encryption_scheme = cdm::EncryptionScheme::kCbcs;
    input.pattern = {crypt, skip};
    input.key_id = key;
    input.key_id_size = 16;
    input.iv = iv;
    input.iv_size = iv_size;
    input.timestamp = timestamp;
    std::vector<cdm::SubsampleEntry> subs;
    for (uint32_t i = 0; i < count; i++) subs.push_back({clear[i], cipher[i]});
    cdm::SubsampleEntry whole{0, length};
    input.subsamples = count ? subs.data() : &whole;
    input.num_subsamples = count ? count : 1;
    VideoFrameImpl frame;
    auto status = ctx->cdm11->DecryptAndDecodeFrame(input, &frame);
    if (status != cdm::kSuccess) return static_cast<int>(status);
    if (!frame.buffer || frame.format != cdm::kI420 || frame.size.width < 1 ||
        frame.size.height < 1 || frame.size.width > 1920 || frame.size.height > 1080)
        return -2;
    // Late pictures still pass through the licensed decoder for dependencies,
    // but avoid a full RGB conversion/copy when catching up or seeking.
    if (frame.timestamp < earliest) return static_cast<int>(cdm::kNeedMoreData);
    auto source = frame.buffer->Data();
    auto size = frame.buffer->Size();
    for (int plane = 0; plane < 3; plane++) {
        uint32_t rows = plane ? (frame.size.height + 1) / 2 : frame.size.height;
        uint32_t columns = plane ? (frame.size.width + 1) / 2 : frame.size.width;
        uint64_t end = static_cast<uint64_t>(frame.offsets[plane]) +
                       static_cast<uint64_t>(rows - 1) * frame.strides[plane] + columns;
        if (frame.strides[plane] < columns || end > size) return -3;
    }
    int w = frame.size.width, h = frame.size.height;
    double scale = std::min(1.0, std::min(1280.0 / w, 720.0 / h));
    *width = std::max(1, static_cast<int>(w * scale));
    *height = std::max(1, static_cast<int>(h * scale));
    *rgb = static_cast<uint8_t*>(std::malloc(static_cast<size_t>(*width) * (*height) * 4));
    if (!*rgb) return -4;
    auto clamp = [](int n) -> uint8_t {
        return static_cast<uint8_t>(std::max(0, std::min(255, n)));
    };
    for (int y = 0; y < *height; y++) {
        int sy = h == *height ? y : y * h / (*height);
        for (int x = 0; x < *width; x++) {
            int sx = w == *width ? x : x * w / (*width);
            int Y = source[frame.offsets[0] + sy * frame.strides[0] + sx] - 16;
            int U = source[frame.offsets[1] + (sy / 2) * frame.strides[1] + sx / 2] - 128;
            int V = source[frame.offsets[2] + (sy / 2) * frame.strides[2] + sx / 2] - 128;
            auto pixel = *rgb + (static_cast<size_t>(y) * (*width) + x) * 4;
            pixel[0] = clamp((298 * Y + 409 * V + 128) >> 8);
            pixel[1] = clamp((298 * Y - 100 * U - 208 * V + 128) >> 8);
            pixel[2] = clamp((298 * Y + 516 * U + 128) >> 8);
            pixel[3] = 255;
        }
    }
    *pts = frame.timestamp;
    return 0;
}
