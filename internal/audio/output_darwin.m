#import <AVFoundation/AVFoundation.h>
#import <Foundation/Foundation.h>
#include "output_darwin.h"
#include <math.h>
#include <string.h>

// LOCAL uses a player node through a real ten-band AVAudioUnitEQ. All calls are
// serialized by engine.Local; no UI run loop or browser is required.
@interface LMFilePlayer : NSObject {
@public
    AVAudioEngine *engine;
    AVAudioPlayerNode *node;
    AVAudioUnitEQ *equalizer;
    AVAudioFile *file;
    double offset;
    BOOL playing;
}
@end
@implementation LMFilePlayer
- (void)dealloc {
    [node stop];
    [engine stop];
    [file release];
    [node release];
    [equalizer release];
    [engine release];
    [super dealloc];
}
@end

static double lm_duration(LMFilePlayer *p) {
    return (double)p->file.length / p->file.processingFormat.sampleRate;
}
static double lm_position(LMFilePlayer *p) {
    AVAudioTime *render = p->node.lastRenderTime;
    AVAudioTime *time = render ? [p->node playerTimeForNodeTime:render] : nil;
    double elapsed = time && time.sampleRate > 0 ? (double)time.sampleTime/time.sampleRate : 0;
    return fmin(lm_duration(p), p->offset + fmax(0, elapsed));
}
static void lm_schedule(LMFilePlayer *p, double seconds) {
    [p->node stop];
    double rate = p->file.processingFormat.sampleRate;
    AVAudioFramePosition start = (AVAudioFramePosition)(fmax(0, fmin(lm_duration(p), seconds))*rate);
    p->offset = (double)start/rate;
    AVAudioFramePosition remaining = p->file.length-start;
    if (remaining > 0) {
        [p->node scheduleSegment:p->file startingFrame:start frameCount:(AVAudioFrameCount)remaining atTime:nil completionHandler:nil];
    }
}
void* lm_audio_open(const char *path, char **message, double *duration) {
    @autoreleasepool {
        NSError *error = nil;
        LMFilePlayer *p = [[LMFilePlayer alloc] init];
        p->file = [[AVAudioFile alloc] initForReading:[NSURL fileURLWithPath:[NSString stringWithUTF8String:path]] error:&error];
        if (!p->file || p->file.processingFormat.sampleRate <= 0 || p->file.length > UINT32_MAX) {
            *message = strdup(error ? error.localizedDescription.UTF8String : "Unsupported or oversized audio file");
            [p release]; return NULL;
        }
        p->engine = [[AVAudioEngine alloc] init];
        p->node = [[AVAudioPlayerNode alloc] init];
        p->equalizer = [[AVAudioUnitEQ alloc] initWithNumberOfBands:10];
        const double frequencies[10] = {32,64,125,250,500,1000,2000,4000,8000,16000};
        for (int i=0; i<10; i++) {
            AVAudioUnitEQFilterParameters *band = p->equalizer.bands[i];
            band.filterType = AVAudioUnitEQFilterTypeParametric;
            band.frequency = fmin(frequencies[i], p->file.processingFormat.sampleRate*0.45);
            band.bandwidth = 1.0;
            band.gain = 0;
            band.bypass = NO;
        }
        [p->engine attachNode:p->node];
        [p->engine attachNode:p->equalizer];
        [p->engine connect:p->node to:p->equalizer format:p->file.processingFormat];
        [p->engine connect:p->equalizer to:p->engine.mainMixerNode format:p->file.processingFormat];
        [p->engine prepare];
        lm_schedule(p, 0);
        *duration = lm_duration(p);
        return p;
    }
}
void lm_audio_close(void *handle) { @autoreleasepool { [(LMFilePlayer*)handle release]; } }
bool lm_audio_play(void *handle) {
    @autoreleasepool {
        LMFilePlayer *p = (LMFilePlayer*)handle;
        if (lm_position(p) >= lm_duration(p)) lm_schedule(p, 0);
        NSError *error = nil;
        if (!p->engine.running && ![p->engine startAndReturnError:&error]) return false;
        [p->node play]; p->playing = YES; return true;
    }
}
void lm_audio_pause(void *handle) { LMFilePlayer *p=(LMFilePlayer*)handle; [p->node pause]; p->playing=NO; }
void lm_audio_seek(void *handle, double seconds) {
    LMFilePlayer *p=(LMFilePlayer*)handle;
    BOOL resume=p->playing;
    lm_schedule(p,seconds);
    if (resume && seconds<lm_duration(p)) [p->node play];
}
void lm_audio_volume(void *handle, double volume) { ((LMFilePlayer*)handle)->node.volume=volume; }
void lm_audio_equalizer(void *handle, const double *gains) {
    LMFilePlayer *p=(LMFilePlayer*)handle;
    for(int i=0;i<10;i++) p->equalizer.bands[i].gain=gains[i];
}
double lm_audio_position(void *handle) { @autoreleasepool { return lm_position((LMFilePlayer*)handle); } }
bool lm_audio_playing(void *handle) {
    LMFilePlayer *p=(LMFilePlayer*)handle;
    return p->playing && lm_position(p)<lm_duration(p);
}
