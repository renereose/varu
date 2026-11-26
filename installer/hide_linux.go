package main

// Linux has no simple call for the FAT hidden attribute, so a stick
// made on Linux hides its clutter only from Linux file managers (.hidden).
func hide(string) error { return nil }

func setVolumeIcon(string) {}
