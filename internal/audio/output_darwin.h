#include <stdbool.h>
void* lm_audio_open(const char*,char**,double*);
void lm_audio_close(void*);
bool lm_audio_play(void*);
void lm_audio_pause(void*);
void lm_audio_seek(void*,double);
void lm_audio_volume(void*,double);
double lm_audio_position(void*);
bool lm_audio_playing(void*);

void lm_audio_equalizer(void*,const double*);
