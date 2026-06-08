//go:build !darwin

package desktop

// Only macOS has an OS-provided About menu item to remove.

func removeNativeAboutMenuItem() {}
