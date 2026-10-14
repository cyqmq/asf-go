package plugins

import "testing"

type testPlugin struct{}

func (testPlugin) Name() string    { return "test" }
func (testPlugin) Version() string { return "1.0.0" }
func (testPlugin) OnLoaded() error { return nil }

type testASFPlugin struct{ testPlugin }

func (testASFPlugin) OnASFInit(json []byte) error { return nil }

type testCommandPlugin struct{ testPlugin }

func (testCommandPlugin) ProcessCommand(botName, message string) (bool, string, error) {
	return true, "handled", nil
}

func TestManagerRegisterAndLoad(t *testing.T) {
	m := NewManager()
	m.Register(testPlugin{})
	m.Register(testASFPlugin{})
	m.Register(testCommandPlugin{})

	if len(m.Plugins()) != 3 {
		t.Fatalf("应注册 3 个插件，得到 %d", len(m.Plugins()))
	}
	if err := m.LoadAll(); err != nil {
		t.Fatal(err)
	}
	if err := m.NotifyASFInit([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	if err := m.NotifyBotInit("bot1"); err != nil {
		t.Fatal(err)
	}
	if err := m.NotifyBotDestroy("bot1"); err != nil {
		t.Fatal(err)
	}
}

func TestCommandPlugin(t *testing.T) {
	m := NewManager()
	m.Register(testCommandPlugin{})
	for _, p := range m.Plugins() {
		if cp, ok := p.(CommandPlugin); ok {
			handled, reply, err := cp.ProcessCommand("bot1", "!status")
			if err != nil {
				t.Fatal(err)
			}
			if !handled || reply != "handled" {
				t.Errorf("命令处理结果不正确: %v %q", handled, reply)
			}
		}
	}
}
