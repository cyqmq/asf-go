package bot

import "testing"

func TestParseGameIdentifier(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		defaultType GameIdentifierType
		wantType    GameIdentifierType
		wantID      uint32
		wantOK      bool
	}{
		{"app 前缀", "app/123", GameIdentifierPackage, GameIdentifierApplication, 123, true},
		{"a 前缀", "a/123", GameIdentifierPackage, GameIdentifierApplication, 123, true},
		{"sub 前缀", "sub/456", GameIdentifierPackage, GameIdentifierPackage, 456, true},
		{"s 前缀", "s/456", GameIdentifierPackage, GameIdentifierPackage, 456, true},
		{"裸数字默认 sub", "789", GameIdentifierPackage, GameIdentifierPackage, 789, true},
		{"裸数字默认 app", "789", GameIdentifierApplication, GameIdentifierApplication, 789, true},
		{"商店 app 链接", "https://store.steampowered.com/app/123/", GameIdentifierPackage, GameIdentifierApplication, 123, true},
		{"商店 sub 链接", "https://store.steampowered.com/sub/456/", GameIdentifierPackage, GameIdentifierPackage, 456, true},
		{"steam launch 链接", "steam://launch/123/", GameIdentifierPackage, GameIdentifierApplication, 123, true},
		{"steam install 链接", "steam://install/123", GameIdentifierPackage, GameIdentifierApplication, 123, true},
		{"无效零值", "app/0", GameIdentifierPackage, 0, 0, false},
		{"非数字", "abc", GameIdentifierPackage, 0, 0, false},
		{"空字符串", "", GameIdentifierPackage, 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotType, gotID, gotOK := ParseGameIdentifier(tc.input, tc.defaultType)
			if gotOK != tc.wantOK {
				t.Fatalf("ParseGameIdentifier(%q) ok = %v, 期望 %v", tc.input, gotOK, tc.wantOK)
			}
			if gotOK && (gotType != tc.wantType || gotID != tc.wantID) {
				t.Errorf("ParseGameIdentifier(%q) = (%d, %d), 期望 (%d, %d)", tc.input, gotType, gotID, tc.wantType, tc.wantID)
			}
		})
	}
}