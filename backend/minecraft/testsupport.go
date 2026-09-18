package minecraft

// SetJavaSearchDirs points runtime discovery at the given directories and
// returns a function that restores the previous setting. It exists so other
// packages can test behaviour that depends on which runtimes are installed.
func SetJavaSearchDirs(dirs ...string) func() {
	original := javaSearchDirs
	javaSearchDirs = dirs
	return func() { javaSearchDirs = original }
}
