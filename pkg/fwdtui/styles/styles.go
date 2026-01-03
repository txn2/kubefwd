package styles

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	ColorYellow  = lipgloss.Color("226")
	ColorBlue    = lipgloss.Color("39")
	ColorGreen   = lipgloss.Color("42")
	ColorRed     = lipgloss.Color("196")
	ColorOrange  = lipgloss.Color("208")
	ColorGray    = lipgloss.Color("240")
	ColorWhite   = lipgloss.Color("255")
	ColorFocused = lipgloss.Color("62")
	ColorCyan    = lipgloss.Color("51")
)

// Header styles
var (
	HeaderTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorYellow)

	HeaderLinkStyle = lipgloss.NewStyle().
			Foreground(ColorBlue).
			Underline(true)

	HeaderVersionStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)
)

// Status styles for port forwards
var (
	StatusActiveStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	StatusConnectingStyle = lipgloss.NewStyle().
				Foreground(ColorYellow)

	StatusErrorStyle = lipgloss.NewStyle().
				Foreground(ColorRed)

	StatusStoppingStyle = lipgloss.NewStyle().
				Foreground(ColorOrange)

	StatusPendingStyle = lipgloss.NewStyle().
				Foreground(ColorGray)
)

// Log level styles
var (
	LogPanicStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRed)

	LogFatalStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorRed)

	LogErrorStyle = lipgloss.NewStyle().
			Foreground(ColorRed)

	LogWarnStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	LogInfoStyle = lipgloss.NewStyle().
			Foreground(ColorGreen)

	LogDebugStyle = lipgloss.NewStyle().
			Foreground(ColorBlue)

	LogTraceStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	LogTimestampStyle = lipgloss.NewStyle().
				Foreground(ColorGray)
)

// Table styles
var (
	TableHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorYellow)

	TableSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(ColorWhite)
)

// Status bar styles
var (
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	StatusBarErrorStyle = lipgloss.NewStyle().
				Foreground(ColorRed)

	StatusBarHelpStyle = lipgloss.NewStyle().
				Foreground(ColorGray)
)

// Help modal styles
var (
	HelpModalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorFocused).
			Padding(1, 2)

	HelpTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorYellow).
			MarginBottom(1)

	HelpKeyStyle = lipgloss.NewStyle().
			Foreground(ColorBlue).
			Width(12)

	HelpDescStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)
)

// Section styles
var (
	SectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorCyan)

	FocusAccentStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)
)

// Detail view styles
var (
	DetailBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorFocused).
				Padding(1, 2)

	DetailLabelStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	DetailValueStyle = lipgloss.NewStyle().
				Foreground(ColorWhite)

	DetailHostnameStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)

	DetailSectionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorBlue).
				MarginTop(1)
)

// Sparkline styles
var (
	SparklineInStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	SparklineOutStyle = lipgloss.NewStyle().
				Foreground(ColorBlue)
)

// HTTP log styles
var (
	HTTPMethodGetStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	HTTPMethodPostStyle = lipgloss.NewStyle().
				Foreground(ColorBlue)

	HTTPMethodPutStyle = lipgloss.NewStyle().
				Foreground(ColorOrange)

	HTTPMethodDeleteStyle = lipgloss.NewStyle().
				Foreground(ColorRed)

	HTTPMethodOtherStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	HTTPStatus2xxStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	HTTPStatus3xxStyle = lipgloss.NewStyle().
				Foreground(ColorYellow)

	HTTPStatus4xxStyle = lipgloss.NewStyle().
				Foreground(ColorOrange)

	HTTPStatus5xxStyle = lipgloss.NewStyle().
				Foreground(ColorRed)

	HTTPPathStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	HTTPTimestampStyle = lipgloss.NewStyle().
				Foreground(ColorGray)

	HTTPDurationStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)
)

// Detail view footer/help styles
var (
	DetailFooterStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				MarginTop(1)

	DetailFooterKeyStyle = lipgloss.NewStyle().
				Foreground(ColorBlue)
)

// Tab styles
var (
	TabActiveStyle = lipgloss.NewStyle().
			Foreground(ColorWhite).
			Background(ColorBlue).
			Bold(true).
			Padding(0, 1)

	TabInactiveStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Padding(0, 1)
)

// Browse modal styles
var (
	BrowseModalStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(ColorFocused).
				Padding(1, 2)

	BrowseTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(ColorYellow)

	BrowseSubtitleStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)

	BrowseHintStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	BrowseItemStyle = lipgloss.NewStyle().
			Foreground(ColorWhite)

	BrowseSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Foreground(ColorWhite)

	BrowseForwardedStyle = lipgloss.NewStyle().
				Foreground(ColorGreen)

	BrowseForwardAllStyle = lipgloss.NewStyle().
				Foreground(ColorBlue).
				Bold(true)

	HeaderContextStyle = lipgloss.NewStyle().
				Foreground(ColorCyan)

	HeaderHintStyle = lipgloss.NewStyle().
			Foreground(ColorGray)
)
