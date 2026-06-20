package styles_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/txn2/kubefwd/pkg/fwdtui/styles"
)

func TestStylesRenderDarkTheme(t *testing.T) {
	styles.SetDarkTheme(true)

	tests := []struct {
		name  string
		style *lipgloss.Style
	}{
		{"HeaderTitle", &styles.HeaderTitleStyle},
		{"StatusActive", &styles.StatusActiveStyle},
		{"StatusError", &styles.StatusErrorStyle},
		{"LogWarn", &styles.LogWarnStyle},
		{"TableHeader", &styles.TableHeaderStyle},
		{"TableSelected", &styles.TableSelectedStyle},
		{"TabActive", &styles.TabActiveStyle},
		{"TabInactive", &styles.TabInactiveStyle},
		{"DetailValue", &styles.DetailValueStyle},
		{"HTTPMethodGet", &styles.HTTPMethodGetStyle},
		{"BrowseSelected", &styles.BrowseSelectedStyle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.style.Render("test")
			if result == "" {
				t.Errorf("style %s rendered empty string", tt.name)
			}
		})
	}
}

func TestStylesRenderLightTheme(t *testing.T) {
	styles.SetDarkTheme(false)
	defer styles.SetDarkTheme(true)

	tests := []struct {
		name  string
		style *lipgloss.Style
	}{
		{"HeaderTitle", &styles.HeaderTitleStyle},
		{"StatusActive", &styles.StatusActiveStyle},
		{"StatusError", &styles.StatusErrorStyle},
		{"LogWarn", &styles.LogWarnStyle},
		{"TableHeader", &styles.TableHeaderStyle},
		{"TableSelected", &styles.TableSelectedStyle},
		{"TabActive", &styles.TabActiveStyle},
		{"TabInactive", &styles.TabInactiveStyle},
		{"DetailValue", &styles.DetailValueStyle},
		{"HTTPMethodGet", &styles.HTTPMethodGetStyle},
		{"BrowseSelected", &styles.BrowseSelectedStyle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.style.Render("test")
			if result == "" {
				t.Errorf("style %s rendered empty string", tt.name)
			}
		})
	}
}

func TestThemeSwitchProducesDifferentOutput(t *testing.T) {
	// In lipgloss v2, Style.Render emits the raw ANSI color codes directly
	// (color-profile downsampling happens at the Writer layer), so escape
	// codes are generated without forcing a profile.
	styles.SetDarkTheme(true)
	darkResult := styles.HeaderTitleStyle.Render("test")

	styles.SetDarkTheme(false)
	lightResult := styles.HeaderTitleStyle.Render("test")

	// Restore
	styles.SetDarkTheme(true)

	if darkResult == lightResult {
		t.Errorf("Theme switch should produce different output.\nDark:  %q\nLight: %q", darkResult, lightResult)
	}

	t.Logf("Dark:  %q", darkResult)
	t.Logf("Light: %q", lightResult)
}

func TestIsDarkTheme(t *testing.T) {
	styles.SetDarkTheme(true)
	if !styles.IsDarkTheme() {
		t.Error("expected dark theme")
	}

	styles.SetDarkTheme(false)
	if styles.IsDarkTheme() {
		t.Error("expected light theme")
	}

	// Restore
	styles.SetDarkTheme(true)
}
