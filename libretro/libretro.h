#ifndef LIBRETRO_H
#define LIBRETRO_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

#ifndef RETRO_CALLCONV
#define RETRO_CALLCONV
#endif

#define RETRO_API_VERSION 1

#define RETRO_DEVICE_JOYPAD 1

#define RETRO_DEVICE_ID_JOYPAD_B      0
#define RETRO_DEVICE_ID_JOYPAD_Y      1
#define RETRO_DEVICE_ID_JOYPAD_SELECT 2
#define RETRO_DEVICE_ID_JOYPAD_START  3
#define RETRO_DEVICE_ID_JOYPAD_UP     4
#define RETRO_DEVICE_ID_JOYPAD_DOWN   5
#define RETRO_DEVICE_ID_JOYPAD_LEFT   6
#define RETRO_DEVICE_ID_JOYPAD_RIGHT  7
#define RETRO_DEVICE_ID_JOYPAD_A      8
#define RETRO_DEVICE_ID_JOYPAD_X      9
#define RETRO_DEVICE_ID_JOYPAD_L      10
#define RETRO_DEVICE_ID_JOYPAD_R      11
#define RETRO_DEVICE_ID_JOYPAD_L2     12
#define RETRO_DEVICE_ID_JOYPAD_R2     13
#define RETRO_DEVICE_ID_JOYPAD_L3     14
#define RETRO_DEVICE_ID_JOYPAD_R3     15

#define RETRO_REGION_NTSC 0
#define RETRO_REGION_PAL  1

#define RETRO_MEMORY_SAVE_RAM   0
#define RETRO_MEMORY_SYSTEM_RAM 2

#define RETRO_ENVIRONMENT_SET_PIXEL_FORMAT      10
#define RETRO_ENVIRONMENT_GET_VARIABLE          15
#define RETRO_ENVIRONMENT_SET_VARIABLES         16
#define RETRO_ENVIRONMENT_GET_VARIABLE_UPDATE   17
#define RETRO_ENVIRONMENT_SET_GEOMETRY          37

enum retro_pixel_format {
	RETRO_PIXEL_FORMAT_0RGB1555 = 0,
	RETRO_PIXEL_FORMAT_XRGB8888 = 1,
	RETRO_PIXEL_FORMAT_RGB565 = 2
};

typedef bool (RETRO_CALLCONV *retro_environment_t)(unsigned cmd, void *data);
typedef void (RETRO_CALLCONV *retro_video_refresh_t)(const void *data, unsigned width, unsigned height, size_t pitch);
typedef void (RETRO_CALLCONV *retro_audio_sample_t)(int16_t left, int16_t right);
typedef size_t (RETRO_CALLCONV *retro_audio_sample_batch_t)(const int16_t *data, size_t frames);
typedef void (RETRO_CALLCONV *retro_input_poll_t)(void);
typedef int16_t (RETRO_CALLCONV *retro_input_state_t)(unsigned port, unsigned device, unsigned index, unsigned id);

struct retro_system_info {
	const char *library_name;
	const char *library_version;
	const char *valid_extensions;
	bool need_fullpath;
	bool block_extract;
};

struct retro_game_geometry {
	unsigned base_width;
	unsigned base_height;
	unsigned max_width;
	unsigned max_height;
	float aspect_ratio;
};

struct retro_system_timing {
	double fps;
	double sample_rate;
};

struct retro_system_av_info {
	struct retro_game_geometry geometry;
	struct retro_system_timing timing;
};

struct retro_variable {
	const char *key;
	const char *value;
};

struct retro_game_info {
	const char *path;
	const void *data;
	size_t size;
	const char *meta;
};

#endif
