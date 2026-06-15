package tool

// RegisterDefaults installs the built-in toolset against ws. This is the single
// place that decides what an out-of-the-box agent can do; trimming or extending
// the capability surface is a one-line change here.
func RegisterDefaults(r *Registry, ws *Workspace) {
	r.Register(&ReadFile{WS: ws})
	r.Register(&WriteFile{WS: ws})
	r.Register(&EditFile{WS: ws})
	r.Register(&ListDir{WS: ws})
	r.Register(&Grep{WS: ws})
	r.Register(&Shell{WS: ws})
}
