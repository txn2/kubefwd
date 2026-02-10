package styles_test

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
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
	// Force ANSI256 color profile so escape codes are actually generated
	lipgloss.SetColorProfile(termenv.ANSI256)

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
