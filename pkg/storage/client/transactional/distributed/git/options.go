package git

type Options struct {
	// default is autodetect, i.e. the clone is made without a branch
	MainBranch string

	// Authentication method. If unspecified, this clone is read-only.
	AuthMethod AuthMethod
}

func defaultOpts() *Options {
	return &Options{}
}

type Option interface {
	ApplyTo(*Options)
}

func (o *Options) ApplyToTx(target *Options) {
	if o.MainBranch != "" {
		target.MainBranch = o.MainBranch
	}
	if o.AuthMethod != nil {
		target.AuthMethod = o.AuthMethod
	}
}

func (o *Options) ApplyOptions(opts []Option) *Options {
	for _, opt := range opts {
		opt.ApplyTo(o)
	}
	return o
}

type Branch string

func (b Branch) ApplyTo(target *Options) {
	if b != "" {
		target.MainBranch = string(b)
	}
}
