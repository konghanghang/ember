package p115

import "testing"

func TestNewCookieProviderComposesCompleteProviderWithoutNetwork(t *testing.T) {
	provider := NewCookieProvider()
	if provider == nil || provider.CookieCredentialValidator == nil || provider.CookieHTTPAdapter == nil {
		t.Fatalf("NewCookieProvider() returned incomplete provider: %+v", provider)
	}
	var _ Provider = provider
}
