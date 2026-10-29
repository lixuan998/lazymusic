//go:build darwin && cgo

package volume

/*
#cgo LDFLAGS: -framework CoreAudio
#include <CoreAudio/CoreAudio.h>
static AudioDeviceID lm_default_output(void){AudioDeviceID d=kAudioObjectUnknown;UInt32 n=sizeof(d);AudioObjectPropertyAddress a={kAudioHardwarePropertyDefaultOutputDevice,kAudioObjectPropertyScopeGlobal,kAudioObjectPropertyElementMain};if(AudioObjectGetPropertyData(kAudioObjectSystemObject,&a,0,NULL,&n,&d)!=noErr)return kAudioObjectUnknown;return d;}
static int lm_volume(float* out,int* muted,float value,int write){
 AudioDeviceID d=lm_default_output();if(d==kAudioObjectUnknown)return -1;
 AudioObjectPropertyAddress a={kAudioDevicePropertyVolumeScalar,kAudioDevicePropertyScopeOutput,kAudioObjectPropertyElementMain};UInt32 n=sizeof(Float32);Float32 v=0;int found=0;float sum=0;
 for(UInt32 channel=0;channel<3;channel++){
  a.mElement=channel;if(!AudioObjectHasProperty(d,&a))continue;
  if(write){Boolean writable=0;if(AudioObjectIsPropertySettable(d,&a,&writable)!=noErr||!writable)continue;v=value;if(AudioObjectSetPropertyData(d,&a,0,NULL,n,&v)!=noErr)continue;}
  else{if(AudioObjectGetPropertyData(d,&a,0,NULL,&n,&v)!=noErr)continue;sum+=v;}
  found++;if(channel==0)break;
 }
 if(!found)return -2;*out=write?value:sum/found;
 a.mSelector=kAudioDevicePropertyMute;a.mElement=kAudioObjectPropertyElementMain;UInt32 mute=0;n=sizeof(mute);
 if(AudioObjectHasProperty(d,&a)){if(write&&value>0){Boolean writable=0;if(AudioObjectIsPropertySettable(d,&a,&writable)==noErr&&writable)AudioObjectSetPropertyData(d,&a,0,NULL,n,&mute);}AudioObjectGetPropertyData(d,&a,0,NULL,&n,&mute);}*muted=(int)mute;return 0;
}
*/
import "C"
import (
	"context"
	"fmt"
)

type driver struct{}

func newDriver() Driver { return driver{} }
func (driver) Read(context.Context) (float64, bool, error) {
	var v C.float
	var m C.int
	if C.lm_volume(&v, &m, 0, 0) != 0 {
		return 0, false, fmt.Errorf("default output device does not expose system volume")
	}
	return float64(v), m != 0, nil
}
func (driver) Write(_ context.Context, value float64) error {
	var v C.float
	var m C.int
	if C.lm_volume(&v, &m, C.float(value), 1) != 0 {
		return fmt.Errorf("default output device does not support changing system volume")
	}
	return nil
}
