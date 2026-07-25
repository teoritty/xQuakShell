package loghub

// LevelController adapts the package-level log level to domain.LogLevelController.
type LevelController struct{}

// SetLevel parses a settings-level name and applies it process-wide.
func (LevelController) SetLevel(name string) { SetLevel(ParseLevel(name)) }
