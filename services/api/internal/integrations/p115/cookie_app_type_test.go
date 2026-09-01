package p115

import "testing"

func TestDetectCookieAppType(t *testing.T) {
	tests := []struct {
		name   string
		cookie string
		want   string
		wantOK bool
	}{
		{name: "web", cookie: "UID=100_A1_1700000000; CID=fake", want: "web", wantOK: true},
		{name: "ios", cookie: "UID=100_D1_1700000000", want: "ios", wantOK: true},
		{name: "ios variant", cookie: "UID=100_D2_1700000000", want: "bios", wantOK: true},
		{name: "115 ios", cookie: "UID=100_D3_1700000000", want: "115ios", wantOK: true},
		{name: "android", cookie: "CID=fake; UID=100_F1_1700000000; SEID=fake", want: "android", wantOK: true},
		{name: "android variant", cookie: "UID=100_F2_1700000000", want: "bandroid", wantOK: true},
		{name: "115 android", cookie: "UID=100_F3_1700000000", want: "115android", wantOK: true},
		{name: "ipad", cookie: "UID=100_H1_1700000000", want: "ipad", wantOK: true},
		{name: "ipad variant", cookie: "UID=100_H2_1700000000", want: "bipad", wantOK: true},
		{name: "115 ipad", cookie: "UID=100_H3_1700000000", want: "115ipad", wantOK: true},
		{name: "android tv", cookie: "UID=100_I1_1700000000", want: "tv", wantOK: true},
		{name: "apple tv", cookie: "UID=100_I2_1700000000", want: "apple_tv", wantOK: true},
		{name: "management android", cookie: "UID=100_M1_1700000000", want: "qandroid", wantOK: true},
		{name: "management ios", cookie: "UID=100_N1_1700000000", want: "qios", wantOK: true},
		{name: "management ipad", cookie: "UID=100_O1_1700000000", want: "qipad", wantOK: true},
		{name: "windows", cookie: "UID=100_P1_1700000000", want: "os_windows", wantOK: true},
		{name: "desktop os", cookie: "UID=100_P2_1700000000", want: "os_mac", wantOK: true},
		{name: "linux", cookie: "UID=100_P3_1700000000", want: "os_linux", wantOK: true},
		{name: "wechat mini", cookie: "UID=100_R1_1700000000", want: "wechatmini", wantOK: true},
		{name: "alipay mini", cookie: "UID=100_R2_1700000000", want: "alipaymini", wantOK: true},
		{name: "harmony", cookie: "UID=100_S1_1700000000", want: "harmony", wantOK: true},
		{name: "case normalized", cookie: "UID=100_f1_1700000000", want: "android", wantOK: true},
		{name: "unknown ssoent", cookie: "UID=100_A2_1700000000"},
		{name: "missing ssoent", cookie: "UID=100"},
		{name: "empty ssoent", cookie: "UID=100__1700000000"},
		{name: "duplicate uid", cookie: "UID=100_A1_1700000000; UID=200_F1_1700000000"},
		{name: "missing uid", cookie: "CID=fake; SEID=fake"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := DetectCookieAppType(tt.cookie)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("DetectCookieAppType() = %q, %t, want %q, %t", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
