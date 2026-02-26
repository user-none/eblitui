package libretro

/*
#include "libretro.h"
#include "cfuncs.h"
*/
import "C"
import (
	"strings"
	"unsafe"

	emucore "github.com/user-none/eblitui/api"
)

// Libretro joypad button ID constants for use in RetropadMapping.
const (
	JoypadB      = C.RETRO_DEVICE_ID_JOYPAD_B
	JoypadY      = C.RETRO_DEVICE_ID_JOYPAD_Y
	JoypadSelect = C.RETRO_DEVICE_ID_JOYPAD_SELECT
	JoypadStart  = C.RETRO_DEVICE_ID_JOYPAD_START
	JoypadA      = C.RETRO_DEVICE_ID_JOYPAD_A
	JoypadX      = C.RETRO_DEVICE_ID_JOYPAD_X
	JoypadL      = C.RETRO_DEVICE_ID_JOYPAD_L
	JoypadR      = C.RETRO_DEVICE_ID_JOYPAD_R
	JoypadL2     = C.RETRO_DEVICE_ID_JOYPAD_L2
	JoypadR2     = C.RETRO_DEVICE_ID_JOYPAD_R2
	JoypadL3     = C.RETRO_DEVICE_ID_JOYPAD_L3
	JoypadR3     = C.RETRO_DEVICE_ID_JOYPAD_R3
)

// RetropadMapping maps a libretro button ID to an emucore bit position.
type RetropadMapping struct {
	RetroID int // RETRO_DEVICE_ID_JOYPAD_* constant
	BitID   int // emucore bit position (from Button.ID)
}

// memoryBuffer holds a C-allocated buffer for a memory region.
type memoryBuffer struct {
	buf  *C.uint8_t
	size C.size_t
}

var (
	factory  emucore.CoreFactory
	inputMap []RetropadMapping
	sysInfo  emucore.SystemInfo

	emulator     emucore.Emulator
	saveStater   emucore.SaveStater
	memoryMapper emucore.MemoryMapper

	region        emucore.Region
	romData       []byte
	xrgbBuf       []byte
	currentWidth  int
	currentHeight int

	// C-allocated memory buffers keyed by region type
	memBuffers map[int]*memoryBuffer

	// Pre-allocated C strings (allocated once, freed in retro_deinit)
	libNameStr   *C.char
	libVerStr    *C.char
	validExtStr  *C.char
	stringsReady bool

	// Core option state
	optionRegion   string = "Auto"
	detectedRegion emucore.Region

	// Pre-allocated C strings for options
	optKeyRegion *C.char
	optValRegion *C.char
	optionPrefix string

	// Core-specific option keys and C strings
	coreOptKeys []*C.char
	coreOptVals []*C.char
)

// RegisterFactory sets the CoreFactory and input mapping used by the libretro core.
// Must be called during init() before any retro_* function runs.
func RegisterFactory(f emucore.CoreFactory, mapping []RetropadMapping) {
	factory = f
	inputMap = mapping
	sysInfo = f.SystemInfo()
	optionPrefix = sysInfo.CoreName + "_"
}

//export retro_set_environment
func retro_set_environment(cb C.retro_environment_t) {
	C._retro_set_environment(cb)
	ensureOptionStrings()
	setVariables()
}

//export retro_set_video_refresh
func retro_set_video_refresh(cb C.retro_video_refresh_t) {
	C._retro_set_video_refresh(cb)
}

//export retro_set_audio_sample
func retro_set_audio_sample(cb C.retro_audio_sample_t) {
	C._retro_set_audio_sample(cb)
}

//export retro_set_audio_sample_batch
func retro_set_audio_sample_batch(cb C.retro_audio_sample_batch_t) {
	C._retro_set_audio_sample_batch(cb)
}

//export retro_set_input_poll
func retro_set_input_poll(cb C.retro_input_poll_t) {
	C._retro_set_input_poll(cb)
}

//export retro_set_input_state
func retro_set_input_state(cb C.retro_input_state_t) {
	C._retro_set_input_state(cb)
}

//export retro_init
func retro_init() {
	maxPixels := sysInfo.ScreenWidth * sysInfo.MaxScreenHeight
	xrgbBuf = make([]byte, maxPixels*4)
	currentWidth = sysInfo.ScreenWidth
	currentHeight = sysInfo.MaxScreenHeight

	ensureStrings()
	ensureOptionStrings()
}

//export retro_deinit
func retro_deinit() {
	emulator = nil
	saveStater = nil
	memoryMapper = nil
	romData = nil
	xrgbBuf = nil

	freeMemBuffers()
}

//export retro_api_version
func retro_api_version() C.uint {
	return C.RETRO_API_VERSION
}

//export retro_get_system_info
func retro_get_system_info(info *C.struct_retro_system_info) {
	ensureStrings()
	info.library_name = libNameStr
	info.library_version = libVerStr
	info.valid_extensions = validExtStr
	info.need_fullpath = C.bool(false)
}

//export retro_get_system_av_info
func retro_get_system_av_info(info *C.struct_retro_system_av_info) {
	var fps int
	if emulator != nil {
		fps = emulator.GetTiming().FPS
	} else {
		fps = 60
	}

	info.timing.fps = C.double(fps)
	info.timing.sample_rate = C.double(sysInfo.SampleRate)

	baseWidth := currentWidth
	if baseWidth == 0 {
		baseWidth = sysInfo.ScreenWidth
	}
	info.geometry.base_width = C.uint(baseWidth)
	info.geometry.base_height = C.uint(sysInfo.MaxScreenHeight)
	info.geometry.max_width = C.uint(sysInfo.ScreenWidth)
	info.geometry.max_height = C.uint(sysInfo.MaxScreenHeight)
	info.geometry.aspect_ratio = C.float(emucore.DisplayAspectRatio(baseWidth, int(sysInfo.MaxScreenHeight), sysInfo.PixelAspectRatio))
}

//export retro_set_controller_port_device
func retro_set_controller_port_device(port C.uint, device C.uint) {
}

//export retro_reset
func retro_reset() {
	if romData == nil || factory == nil {
		return
	}

	applyRegionOption()

	emu, err := factory.CreateEmulator(romData, region)
	if err != nil {
		return
	}
	setEmulator(emu)
}

//export retro_run
func retro_run() {
	if emulator == nil {
		return
	}

	// Check for option changes
	var updated C.bool
	if C.call_environ_cb(C.RETRO_ENVIRONMENT_GET_VARIABLE_UPDATE, unsafe.Pointer(&updated)) && updated {
		updateCoreOptions()
	}

	// Poll input
	C.call_input_poll_cb()

	// Build input bitmask for each player
	for player := 0; player < sysInfo.Players; player++ {
		port := C.uint(player)
		var buttons uint32

		// D-pad (fixed mapping)
		if C.call_input_state_cb(port, C.RETRO_DEVICE_JOYPAD, 0, C.RETRO_DEVICE_ID_JOYPAD_UP) != 0 {
			buttons |= 1 << emucore.ButtonUp
		}
		if C.call_input_state_cb(port, C.RETRO_DEVICE_JOYPAD, 0, C.RETRO_DEVICE_ID_JOYPAD_DOWN) != 0 {
			buttons |= 1 << emucore.ButtonDown
		}
		if C.call_input_state_cb(port, C.RETRO_DEVICE_JOYPAD, 0, C.RETRO_DEVICE_ID_JOYPAD_LEFT) != 0 {
			buttons |= 1 << emucore.ButtonLeft
		}
		if C.call_input_state_cb(port, C.RETRO_DEVICE_JOYPAD, 0, C.RETRO_DEVICE_ID_JOYPAD_RIGHT) != 0 {
			buttons |= 1 << emucore.ButtonRight
		}

		// System-specific buttons via input mapping
		for _, m := range inputMap {
			if C.call_input_state_cb(port, C.RETRO_DEVICE_JOYPAD, 0, C.uint(m.RetroID)) != 0 {
				buttons |= 1 << uint(m.BitID)
			}
		}

		emulator.SetInput(player, buttons)
	}

	// Sync save RAM from C buffer to Go before frame
	if memoryMapper != nil {
		if buf, ok := memBuffers[emucore.MemorySaveRAM]; ok && buf.buf != nil {
			data := unsafe.Slice((*byte)(unsafe.Pointer(buf.buf)), int(buf.size))
			goData := make([]byte, int(buf.size))
			copy(goData, data)
			memoryMapper.WriteRegion(emucore.MemorySaveRAM, goData)
		}
	}

	// Run one frame
	emulator.RunFrame()

	// Sync Go memory to C buffers after frame
	if memoryMapper != nil {
		for regionType, buf := range memBuffers {
			if buf.buf == nil {
				continue
			}
			regionData := memoryMapper.ReadRegion(regionType)
			if len(regionData) > 0 {
				cBuf := unsafe.Slice((*byte)(unsafe.Pointer(buf.buf)), int(buf.size))
				copy(cBuf, regionData)
			}
		}
	}

	// Video output
	fb := emulator.GetFramebuffer()
	activeHeight := emulator.GetActiveHeight()
	if len(fb) > 0 {
		outputVideo(fb, activeHeight)
	}

	// Audio output
	samples := emulator.GetAudioSamples()
	if len(samples) > 0 {
		frames := len(samples) / 2
		C.call_audio_batch_cb((*C.int16_t)(unsafe.Pointer(&samples[0])), C.size_t(frames))
	}
}

//export retro_serialize_size
func retro_serialize_size() C.size_t {
	return C.size_t(sysInfo.SerializeSize)
}

//export retro_serialize
func retro_serialize(data unsafe.Pointer, size C.size_t) C.bool {
	if saveStater == nil {
		return C.bool(false)
	}

	state, err := saveStater.Serialize()
	if err != nil {
		return C.bool(false)
	}

	if len(state) > int(size) {
		return C.bool(false)
	}

	dst := unsafe.Slice((*byte)(data), size)
	copy(dst, state)
	return C.bool(true)
}

//export retro_unserialize
func retro_unserialize(data unsafe.Pointer, size C.size_t) C.bool {
	if saveStater == nil {
		return C.bool(false)
	}

	state := make([]byte, size)
	src := unsafe.Slice((*byte)(data), size)
	copy(state, src)

	if err := saveStater.Deserialize(state); err != nil {
		return C.bool(false)
	}
	return C.bool(true)
}

//export retro_cheat_reset
func retro_cheat_reset() {
}

//export retro_cheat_set
func retro_cheat_set(index C.uint, enabled C.bool, code *C.char) {
}

//export retro_load_game
func retro_load_game(game *C.struct_retro_game_info) C.bool {
	if game == nil || game.data == nil || game.size == 0 || factory == nil {
		return C.bool(false)
	}

	// Set pixel format
	var pixelFormat C.int = C.RETRO_PIXEL_FORMAT_XRGB8888
	C.call_environ_cb(C.RETRO_ENVIRONMENT_SET_PIXEL_FORMAT, unsafe.Pointer(&pixelFormat))

	// Copy ROM data
	romData = C.GoBytes(game.data, C.int(game.size))

	// Detect region
	detectedRegion, _ = factory.DetectRegion(romData)
	region = detectedRegion

	// Read initial options
	updateCoreOptions()

	// Create emulator
	emu, err := factory.CreateEmulator(romData, region)
	if err != nil {
		return C.bool(false)
	}
	setEmulator(emu)

	// Allocate memory buffers
	allocMemBuffers()

	return C.bool(true)
}

//export retro_load_game_special
func retro_load_game_special(gameType C.uint, info *C.struct_retro_game_info, numInfo C.size_t) C.bool {
	return C.bool(false)
}

//export retro_unload_game
func retro_unload_game() {
	emulator = nil
	saveStater = nil
	memoryMapper = nil
	romData = nil
	freeMemBuffers()
}

//export retro_get_region
func retro_get_region() C.uint {
	if region == emucore.RegionPAL {
		return C.RETRO_REGION_PAL
	}
	return C.RETRO_REGION_NTSC
}

//export retro_get_memory_data
func retro_get_memory_data(id C.uint) unsafe.Pointer {
	retroToEmu := map[C.uint]int{
		C.RETRO_MEMORY_SAVE_RAM:   emucore.MemorySaveRAM,
		C.RETRO_MEMORY_SYSTEM_RAM: emucore.MemorySystemRAM,
	}

	if emuType, ok := retroToEmu[id]; ok {
		if buf, ok := memBuffers[emuType]; ok {
			return unsafe.Pointer(buf.buf)
		}
	}
	return nil
}

//export retro_get_memory_size
func retro_get_memory_size(id C.uint) C.size_t {
	retroToEmu := map[C.uint]int{
		C.RETRO_MEMORY_SAVE_RAM:   emucore.MemorySaveRAM,
		C.RETRO_MEMORY_SYSTEM_RAM: emucore.MemorySystemRAM,
	}

	if emuType, ok := retroToEmu[id]; ok {
		if buf, ok := memBuffers[emuType]; ok {
			return buf.size
		}
	}
	return 0
}

// setEmulator sets the emulator and detects optional interface support.
func setEmulator(emu emucore.Emulator) {
	emulator = emu

	if ss, ok := emu.(emucore.SaveStater); ok {
		saveStater = ss
	} else {
		saveStater = nil
	}

	if mm, ok := emu.(emucore.MemoryMapper); ok {
		memoryMapper = mm
	} else {
		memoryMapper = nil
	}
}

// ensureStrings allocates C strings for system info once.
func ensureStrings() {
	if stringsReady {
		return
	}
	libNameStr = C.CString(sysInfo.CoreName)
	libVerStr = C.CString(sysInfo.CoreVersion)
	validExtStr = C.CString(strings.Join(sysInfo.Extensions, "|"))
	stringsReady = true
}

// ensureOptionStrings allocates C strings for core options once.
func ensureOptionStrings() {
	if optKeyRegion != nil {
		return
	}
	optKeyRegion = C.CString(optionPrefix + "region")
	optValRegion = C.CString("Region; Auto|NTSC|PAL")

	// Build core-specific option strings
	for _, opt := range sysInfo.CoreOptions {
		coreOptKeys = append(coreOptKeys, C.CString(optionPrefix+opt.Key))

		var valStr string
		switch opt.Type {
		case emucore.CoreOptionBool:
			if opt.Default == "true" {
				valStr = opt.Label + "; true|false"
			} else {
				valStr = opt.Label + "; false|true"
			}
		case emucore.CoreOptionSelect:
			ordered := reorderDefault(opt.Values, opt.Default)
			valStr = opt.Label + "; " + strings.Join(ordered, "|")
		default:
			valStr = opt.Label + "; " + strings.Join(opt.Values, "|")
		}
		coreOptVals = append(coreOptVals, C.CString(valStr))
	}
}

// setVariables registers all core options with the frontend.
func setVariables() {
	// Count: region + core options + terminator
	count := 1 // region
	count += len(coreOptKeys)
	count++ // nil terminator

	options := make([]C.struct_retro_variable, count)
	idx := 0

	options[idx] = C.struct_retro_variable{key: optKeyRegion, value: optValRegion}
	idx++

	for i := range coreOptKeys {
		options[idx] = C.struct_retro_variable{key: coreOptKeys[i], value: coreOptVals[i]}
		idx++
	}

	// Nil terminator
	options[idx] = C.struct_retro_variable{key: nil, value: nil}

	C.call_environ_cb(C.RETRO_ENVIRONMENT_SET_VARIABLES, unsafe.Pointer(&options[0]))
}

// updateCoreOptions reads core options from the frontend.
func updateCoreOptions() {
	// Region option
	var regionVar C.struct_retro_variable
	regionVar.key = optKeyRegion
	if C.call_environ_cb(C.RETRO_ENVIRONMENT_GET_VARIABLE, unsafe.Pointer(&regionVar)) && regionVar.value != nil {
		newRegion := C.GoString(regionVar.value)
		if newRegion != optionRegion {
			optionRegion = newRegion
			applyRegionOption()
		}
	}

	// Core-specific options
	for i, cKey := range coreOptKeys {
		var v C.struct_retro_variable
		v.key = cKey
		if C.call_environ_cb(C.RETRO_ENVIRONMENT_GET_VARIABLE, unsafe.Pointer(&v)) && v.value != nil {
			if emulator != nil && i < len(sysInfo.CoreOptions) {
				emulator.SetOption(sysInfo.CoreOptions[i].Key, C.GoString(v.value))
			}
		}
	}
}

// applyRegionOption applies the current region option setting.
func applyRegionOption() {
	var newRegion emucore.Region
	switch optionRegion {
	case "NTSC":
		newRegion = emucore.RegionNTSC
	case "PAL":
		newRegion = emucore.RegionPAL
	default:
		newRegion = detectedRegion
	}
	if newRegion != region {
		region = newRegion
		if emulator != nil {
			emulator.SetRegion(region)
		}
	}
}

// convertRGBAToXRGB8888 converts RGBA pixels to XRGB8888 format.
func convertRGBAToXRGB8888(src, dst []byte, pixels int) {
	for i := 0; i < pixels; i++ {
		srcIdx := i * 4
		dstIdx := i * 4
		dst[dstIdx+0] = src[srcIdx+2] // B
		dst[dstIdx+1] = src[srcIdx+1] // G
		dst[dstIdx+2] = src[srcIdx+0] // R
		dst[dstIdx+3] = 0xFF          // X
	}
}

// outputVideo outputs video to the libretro frontend.
func outputVideo(fb []byte, activeHeight int) {
	screenWidth := sysInfo.ScreenWidth
	pixels := screenWidth * activeHeight
	convertRGBAToXRGB8888(fb, xrgbBuf, pixels)
	C.call_video_cb(unsafe.Pointer(&xrgbBuf[0]), C.uint(screenWidth), C.uint(activeHeight), C.size_t(screenWidth*4))
	if currentWidth != screenWidth || currentHeight != activeHeight {
		currentWidth = screenWidth
		currentHeight = activeHeight
		updateGeometry()
	}
}

// updateGeometry notifies the frontend of geometry changes.
func updateGeometry() {
	var geom C.struct_retro_game_geometry
	geom.base_width = C.uint(currentWidth)
	geom.base_height = C.uint(currentHeight)
	geom.max_width = C.uint(sysInfo.ScreenWidth)
	geom.max_height = C.uint(sysInfo.MaxScreenHeight)
	geom.aspect_ratio = C.float(emucore.DisplayAspectRatio(currentWidth, currentHeight, sysInfo.PixelAspectRatio))
	C.call_environ_cb(C.RETRO_ENVIRONMENT_SET_GEOMETRY, unsafe.Pointer(&geom))
}

// allocMemBuffers allocates C-side memory buffers based on MemoryMapper.
func allocMemBuffers() {
	freeMemBuffers()
	if memoryMapper == nil {
		return
	}

	memBuffers = make(map[int]*memoryBuffer)
	for _, r := range memoryMapper.MemoryMap() {
		buf := (*C.uint8_t)(C.malloc(C.size_t(r.Size)))
		C.memset(unsafe.Pointer(buf), 0, C.size_t(r.Size))
		memBuffers[r.Type] = &memoryBuffer{buf: buf, size: C.size_t(r.Size)}
	}
}

// freeMemBuffers frees all C-allocated memory buffers.
func freeMemBuffers() {
	for _, buf := range memBuffers {
		if buf.buf != nil {
			C.free(unsafe.Pointer(buf.buf))
		}
	}
	memBuffers = nil
}

// reorderDefault moves the default value to the front of a values slice.
func reorderDefault(values []string, def string) []string {
	result := make([]string, 0, len(values))
	result = append(result, def)
	for _, v := range values {
		if v != def {
			result = append(result, v)
		}
	}
	return result
}
