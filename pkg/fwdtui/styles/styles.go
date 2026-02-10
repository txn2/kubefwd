package styles

import "github.com/charmbracelet/lipgloss"

// color returns a lipgloss.Color, choosing light or dark variant based on the
// current theme set by SetDarkTheme.
func color(light, dark string) lipgloss.Color {
	if isDark {
		return lipgloss.Color(dark)
	}
	return lipgloss.Color(light)
}

// isDark tracks the current theme. Default is dark (original behavior).
var isDark = true

// SetDarkTheme switches the color palette. Call this before the TUI starts.
// Passing false selects the light palette; true selects the dark palette.
func SetDarkTheme(dark bool) {
	isDark = dark
	applyTheme()
}

// IsDarkTheme returns the current theme setting.
func IsDarkTheme() bool {
	return isDark
}

func applyTheme() {
	// --- palette ---
	colorYellow := color("136", "226")
	colorBlue := color("27", "39")
	colorGreen := color("28", "42")
	colorRed := color("160", "196")
	colorOrange := color("166", "208")
	colorGray := color("243", "240")
	colorWhite := color("16", "255")
	colorFocused := color("62", "62")
	colorCyan := color("30", "51")
	colorSelectedBg := color("254", "237")
	colorTabActiveFg := color("255", "255")

	// --- header ---
	HeaderTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	HeaderLinkStyle = lipgloss.NewStyle().Foreground(colorBlue).Underline(true)
	HeaderVersionStyle = lipgloss.NewStyle().Foreground(colorWhite)

	// --- status ---
	StatusActiveStyle = lipgloss.NewStyle().Foreground(colorGreen)
	StatusConnectingStyle = lipgloss.NewStyle().Foreground(colorYellow)
	StatusErrorStyle = lipgloss.NewStyle().Foreground(colorRed)
	StatusStoppingStyle = lipgloss.NewStyle().Foreground(colorOrange)
	StatusPendingStyle = lipgloss.NewStyle().Foreground(colorGray)

	// --- log levels ---
	LogPanicStyle = lipgloss.NewStyle().Bold(true).Foreground(colorRed)
	LogFatalStyle = lipgloss.NewStyle().Bold(true).Foreground(colorRed)
	LogErrorStyle = lipgloss.NewStyle().Foreground(colorRed)
	LogWarnStyle = lipgloss.NewStyle().Foreground(colorYellow)
	LogInfoStyle = lipgloss.NewStyle().Foreground(colorGreen)
	LogDebugStyle = lipgloss.NewStyle().Foreground(colorBlue)
	LogTraceStyle = lipgloss.NewStyle().Foreground(colorGray)
	LogTimestampStyle = lipgloss.NewStyle().Foreground(colorGray)

	// --- table ---
	TableHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	TableSelectedStyle = lipgloss.NewStyle().Background(colorSelectedBg).Foreground(colorWhite)

	// --- status bar ---
	StatusBarStyle = lipgloss.NewStyle().Foreground(colorWhite)
	StatusBarErrorStyle = lipgloss.NewStyle().Foreground(colorRed)
	StatusBarHelpStyle = lipgloss.NewStyle().Foreground(colorGray)

	// --- help modal ---
	HelpModalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorFocused).Padding(1, 2)
	HelpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorYellow).MarginBottom(1)
	HelpKeyStyle = lipgloss.NewStyle().Foreground(colorBlue).Width(12)
	HelpDescStyle = lipgloss.NewStyle().Foreground(colorWhite)

	// --- section ---
	SectionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorCyan)
	FocusAccentStyle = lipgloss.NewStyle().Foreground(colorCyan)

	// --- detail view ---
	DetailBorderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorFocused).Padding(1, 2)
	DetailLabelStyle = lipgloss.NewStyle().Foreground(colorGray)
	DetailValueStyle = lipgloss.NewStyle().Foreground(colorWhite)
	DetailHostnameStyle = lipgloss.NewStyle().Foreground(colorCyan)
	DetailSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(colorBlue).MarginTop(1)

	// --- sparkline ---
	SparklineInStyle = lipgloss.NewStyle().Foreground(colorGreen)
	SparklineOutStyle = lipgloss.NewStyle().Foreground(colorBlue)

	// --- HTTP log ---
	HTTPMethodGetStyle = lipgloss.NewStyle().Foreground(colorGreen)
	HTTPMethodPostStyle = lipgloss.NewStyle().Foreground(colorBlue)
	HTTPMethodPutStyle = lipgloss.NewStyle().Foreground(colorOrange)
	HTTPMethodDeleteStyle = lipgloss.NewStyle().Foreground(colorRed)
	HTTPMethodOtherStyle = lipgloss.NewStyle().Foreground(colorGray)
	HTTPStatus2xxStyle = lipgloss.NewStyle().Foreground(colorGreen)
	HTTPStatus3xxStyle = lipgloss.NewStyle().Foreground(colorYellow)
	HTTPStatus4xxStyle = lipgloss.NewStyle().Foreground(colorOrange)
	HTTPStatus5xxStyle = lipgloss.NewStyle().Foreground(colorRed)
	HTTPPathStyle = lipgloss.NewStyle().Foreground(colorWhite)
	HTTPTimestampStyle = lipgloss.NewStyle().Foreground(colorGray)
	HTTPDurationStyle = lipgloss.NewStyle().Foreground(colorCyan)

	// --- detail footer ---
	DetailFooterStyle = lipgloss.NewStyle().Foreground(colorGray).MarginTop(1)
	DetailFooterKeyStyle = lipgloss.NewStyle().Foreground(colorBlue)

	// --- tabs ---
	TabActiveStyle = lipgloss.NewStyle().Foreground(colorTabActiveFg).Background(colorBlue).Bold(true).Padding(0, 1)
	TabInactiveStyle = lipgloss.NewStyle().Foreground(colorGray).Padding(0, 1)

	// --- browse modal ---
	BrowseModalStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorFocused).Padding(1, 2)
	BrowseTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(colorYellow)
	BrowseSubtitleStyle = lipgloss.NewStyle().Foreground(colorCyan)
	BrowseHintStyle = lipgloss.NewStyle().Foreground(colorGray)
	BrowseItemStyle = lipgloss.NewStyle().Foreground(colorWhite)
	BrowseSelectedStyle = lipgloss.NewStyle().Background(colorSelectedBg).Foreground(colorWhite)
	BrowseForwardedStyle = lipgloss.NewStyle().Foreground(colorGreen)
	BrowseForwardAllStyle = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	HeaderContextStyle = lipgloss.NewStyle().Foreground(colorCyan)
	HeaderHintStyle = lipgloss.NewStyle().Foreground(colorGray)
}

// All style variables — initialized with dark theme (default).
var (
	// Header styles
	HeaderTitleStyle   lipgloss.Style
	HeaderLinkStyle    lipgloss.Style
	HeaderVersionStyle lipgloss.Style

	// Status styles for port forwards
	StatusActiveStyle     lipgloss.Style
	StatusConnectingStyle lipgloss.Style
	StatusErrorStyle      lipgloss.Style
	StatusStoppingStyle   lipgloss.Style
	StatusPendingStyle    lipgloss.Style

	// Log level styles
	LogPanicStyle     lipgloss.Style
	LogFatalStyle     lipgloss.Style
	LogErrorStyle     lipgloss.Style
	LogWarnStyle      lipgloss.Style
	LogInfoStyle      lipgloss.Style
	LogDebugStyle     lipgloss.Style
	LogTraceStyle     lipgloss.Style
	LogTimestampStyle lipgloss.Style

	// Table styles
	TableHeaderStyle   lipgloss.Style
	TableSelectedStyle lipgloss.Style

	// Status bar styles
	StatusBarStyle      lipgloss.Style
	StatusBarErrorStyle lipgloss.Style
	StatusBarHelpStyle  lipgloss.Style

	// Help modal styles
	HelpModalStyle lipgloss.Style
	HelpTitleStyle lipgloss.Style
	HelpKeyStyle   lipgloss.Style
	HelpDescStyle  lipgloss.Style

	// Section styles
	SectionTitleStyle lipgloss.Style
	FocusAccentStyle  lipgloss.Style

	// Detail view styles
	DetailBorderStyle   lipgloss.Style
	DetailLabelStyle    lipgloss.Style
	DetailValueStyle    lipgloss.Style
	DetailHostnameStyle lipgloss.Style
	DetailSectionStyle  lipgloss.Style

	// Sparkline styles
	SparklineInStyle  lipgloss.Style
	SparklineOutStyle lipgloss.Style

	// HTTP log styles
	HTTPMethodGetStyle    lipgloss.Style
	HTTPMethodPostStyle   lipgloss.Style
	HTTPMethodPutStyle    lipgloss.Style
	HTTPMethodDeleteStyle lipgloss.Style
	HTTPMethodOtherStyle  lipgloss.Style
	HTTPStatus2xxStyle    lipgloss.Style
	HTTPStatus3xxStyle    lipgloss.Style
	HTTPStatus4xxStyle    lipgloss.Style
	HTTPStatus5xxStyle    lipgloss.Style
	HTTPPathStyle         lipgloss.Style
	HTTPTimestampStyle    lipgloss.Style
	HTTPDurationStyle     lipgloss.Style

	// Detail view footer/help styles
	DetailFooterStyle    lipgloss.Style
	DetailFooterKeyStyle lipgloss.Style

	// Tab styles
	TabActiveStyle   lipgloss.Style
	TabInactiveStyle lipgloss.Style

	// Browse modal styles
	BrowseModalStyle      lipgloss.Style
	BrowseTitleStyle      lipgloss.Style
	BrowseSubtitleStyle   lipgloss.Style
	BrowseHintStyle       lipgloss.Style
	BrowseItemStyle       lipgloss.Style
	BrowseSelectedStyle   lipgloss.Style
	BrowseForwardedStyle  lipgloss.Style
	BrowseForwardAllStyle lipgloss.Style
	HeaderContextStyle    lipgloss.Style
	HeaderHintStyle       lipgloss.Style
)

func init() {
	applyTheme()
}
