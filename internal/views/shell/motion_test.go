package shell

import (
	"testing"

	"aurink/internal/contract"
	"github.com/egoist/mygo/ui"
)

type fixedSettings struct{ v contract.Settings }

func (f *fixedSettings) Get() contract.Settings { return f.v }
func (f *fixedSettings) Update(fn func(*contract.Settings)) error {
	fn(&f.v)
	return nil
}
func (f *fixedSettings) Subscribe(func(contract.Settings)) func() { return func() {} }

// Appearance.ReduceMotion "on" and "off" override the system; "system"
// follows it (D18).
func TestReduceMotionSetting(t *testing.T) {
	for _, tc := range []struct {
		setting string
		system  bool
		want    bool
	}{
		{"on", false, true}, {"off", true, false}, {"system", true, true}, {"system", false, false},
	} {
		st := &fixedSettings{}
		st.v.Appearance.ReduceMotion = tc.setting
		s := &Shell{o: Options{Settings: st}}
		var got bool
		tt := ui.NewTester(func(c *ui.Context) { got = s.reduceMotion(c) }, 100, 100)
		tt.SetPreferences(ui.Preferences{ReduceMotion: tc.system})
		tt.Frame()
		if got != tc.want {
			t.Errorf("setting %q, system %v: reduced %v", tc.setting, tc.system, got)
		}
	}
}
