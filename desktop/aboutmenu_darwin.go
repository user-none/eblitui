//go:build darwin

package desktop

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
void removeAboutMenuItem(void);
*/
import "C"

// removeNativeAboutMenuItem removes the macOS application menu's About item.
// The in-app About (settings) is the only About. Safe to call once at startup;
// the menu work is deferred until the menu exists.
func removeNativeAboutMenuItem() {
	C.removeAboutMenuItem()
}
