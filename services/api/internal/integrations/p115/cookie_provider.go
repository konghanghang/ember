package p115

// CookieProvider composes credential validation and Cookie/Web HTTP operations
// into the complete Provider contract consumed by playback services.
type CookieProvider struct {
	*CookieCredentialValidator
	*CookieHTTPAdapter
}

// NewCookieProvider builds the complete production Cookie Provider without performing a network call.
func NewCookieProvider() *CookieProvider {
	client := newCookieHTTPClient()
	validator, err := newCookieCredentialValidator(client, cookieLoginStatusURL)
	if err != nil {
		panic("invalid fixed 115 login status URL: " + err.Error())
	}
	adapter, err := newCookieHTTPAdapter(client, cookieUploadInfoURL, cookieSHASearchURL, cookieUploadInitURL)
	if err != nil {
		panic("invalid fixed 115 Cookie HTTP endpoint: " + err.Error())
	}
	return &CookieProvider{
		CookieCredentialValidator: validator,
		CookieHTTPAdapter:         adapter,
	}
}

var _ Provider = (*CookieProvider)(nil)
