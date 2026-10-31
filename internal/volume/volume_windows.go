//go:build windows && cgo

package volume

/*
#cgo LDFLAGS: -lole32 -luuid
#define COBJMACROS
#include <windows.h>
#include <initguid.h>
#include <mmdeviceapi.h>
#include <endpointvolume.h>

static HRESULT lm_volume(float* level, BOOL* muted, int write) {
 HRESULT init = CoInitializeEx(NULL, COINIT_MULTITHREADED);
 if (FAILED(init) && init != RPC_E_CHANGED_MODE) return init;
 IMMDeviceEnumerator* enumerator = NULL;
 IMMDevice* device = NULL;
 IAudioEndpointVolume* volume = NULL;
 HRESULT hr = CoCreateInstance(&CLSID_MMDeviceEnumerator, NULL, CLSCTX_INPROC_SERVER, &IID_IMMDeviceEnumerator, (void**)&enumerator);
 if (SUCCEEDED(hr)) hr = IMMDeviceEnumerator_GetDefaultAudioEndpoint(enumerator, eRender, eMultimedia, &device);
 if (SUCCEEDED(hr)) hr = IMMDevice_Activate(device, &IID_IAudioEndpointVolume, CLSCTX_INPROC_SERVER, NULL, (void**)&volume);
 if (SUCCEEDED(hr)) {
  if (write) {
   hr = IAudioEndpointVolume_SetMasterVolumeLevelScalar(volume, *level, NULL);
   if (SUCCEEDED(hr) && *level > 0) hr = IAudioEndpointVolume_SetMute(volume, FALSE, NULL);
  } else {
   hr = IAudioEndpointVolume_GetMasterVolumeLevelScalar(volume, level);
   if (SUCCEEDED(hr)) hr = IAudioEndpointVolume_GetMute(volume, muted);
  }
 }
 if (volume) IAudioEndpointVolume_Release(volume);
 if (device) IMMDevice_Release(device);
 if (enumerator) IMMDeviceEnumerator_Release(enumerator);
 if (SUCCEEDED(init)) CoUninitialize();
 return hr;
}
*/
import "C"
import (
	"context"
	"fmt"
	"runtime"
)

type driver struct{}

func newDriver() Driver { return driver{} }
func (driver) Read(ctx context.Context) (float64, bool, error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var level C.float
	var muted C.BOOL
	if hr := C.lm_volume(&level, &muted, 0); hr < 0 {
		return 0, false, fmt.Errorf("default output volume unavailable (HRESULT 0x%08x)", uint32(hr))
	}
	return float64(level), muted != 0, nil
}
func (driver) Write(ctx context.Context, value float64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	level := C.float(value)
	var muted C.BOOL
	if hr := C.lm_volume(&level, &muted, 1); hr < 0 {
		return fmt.Errorf("cannot set default output volume (HRESULT 0x%08x)", uint32(hr))
	}
	return nil
}
