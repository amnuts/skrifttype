//go:build !windows

package winfont

type stubGDI struct{}

func newPlatformGDI() GDI { return &stubGDI{} }

func (s *stubGDI) AddFontResource(path string) (FaceCount, error) {
	return 0, ErrUnsupportedPlatform
}

func (s *stubGDI) RemoveFontResource(path string) error {
	return ErrUnsupportedPlatform
}

func (s *stubGDI) BroadcastFontChange() error {
	return ErrUnsupportedPlatform
}

type stubSystemFontEnumerator struct{}

func newPlatformSystemFontEnumerator() SystemFontEnumerator {
	return &stubSystemFontEnumerator{}
}

func (e *stubSystemFontEnumerator) EnumerateSystemFonts() ([]SystemFont, error) {
	return nil, ErrUnsupportedPlatform
}
