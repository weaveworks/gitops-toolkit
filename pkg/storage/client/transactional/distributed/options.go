package distributed

import "time"

// ClientOption is an interface for applying options to ClientOptions.
type ClientOption interface {
	ApplyToClient(*ClientOptions)
}

// ClientOptions specify options on how the distributed client should
// act according to the PACELC theorem.
//
// The following configurations correspond to the PACELC levels:
//
// PC/EC: CacheValidDuration == 0 && RemoteErrorStream == nil:
// 		This makes every read first do a remote Pull(), and fails
//		critically if the Pull operation fails. Transactions fail
//		if Push() fails.
//
// PC/EL: CacheValidDuration > 0 && RemoteErrorStream == nil:
// 		This makes a read do a remote Pull only if the delta between
// 		the last Pull and time.Now() exceeds CacheValidDuration.
// 		StartResyncLoop(resyncCacheInterval) can be used to
// 		periodically Pull in the background, so that the latency
//		of reads are minimal. Transactions and reads fail if
// 		Push() or Pull() fail.
//
// PA/EL: RemoteErrorStream != nil:
//		How often reads invoke Pull() is given by CacheValidDuration
// 		and StartResyncLoop(resyncCacheInterval) as per above.
//		However, when a Pull() or Push() is invoked from a read or
//		transaction, and a network partition happens, such errors are
//		non-critical for the operation to succeed, as Availability is
//		favored and cached objects are returned.
type ClientOptions struct {
	// CacheValidDuration is the period of time the cache is still
	// valid since its last resync (remote Pull). If set to 0; all
	// reads will invoke a resync right before reading; as the cache
	// is never valid. This option set to 0 favors Consistency over
	// Availability.
	//
	// CacheValidDuration == 0 and RemoteErrorStream != nil must not
	// be set at the same time; as they contradict.
	//
	// Default: 1m
	CacheValidDuration time.Duration
	// RemoteErrorStream specifies a stream in which to readirect
	// errors from the remote, instead of returning them to the caller.
	// This is useful for allowing "offline operation", and favoring
	// Availability over Consistency when a Partition happens (i.e.
	// the network is unreachable). In normal operation, remote Push/Pull
	// errors would propagate to the caller and "fail" the Transaction,
	// however, if that is not desired, those errors can be propagated
	// here, and the caller will succeed with the transaction.
	// Default: nil (optional)
	RemoteErrorStream chan error

	// Default: 30s for all
	LockTimeout time.Duration
	PullTimeout time.Duration
	PushTimeout time.Duration
}

func (o *ClientOptions) ApplyToClient(target *ClientOptions) {
	if o.CacheValidDuration != 0 {
		target.CacheValidDuration = o.CacheValidDuration
	}
	if o.RemoteErrorStream != nil {
		target.RemoteErrorStream = o.RemoteErrorStream
	}
	if o.LockTimeout != 0 {
		target.LockTimeout = o.LockTimeout
	}
	if o.PullTimeout != 0 {
		target.PullTimeout = o.PullTimeout
	}
	if o.PushTimeout != 0 {
		target.PushTimeout = o.PushTimeout
	}
}

func (o *ClientOptions) ApplyOptions(opts []ClientOption) *ClientOptions {
	for _, opt := range opts {
		opt.ApplyToClient(o)
	}
	return o
}

func defaultOptions() *ClientOptions {
	return &ClientOptions{
		CacheValidDuration: 1 * time.Minute,
		RemoteErrorStream:  nil,
		LockTimeout:        30 * time.Second,
		PullTimeout:        30 * time.Second,
		PushTimeout:        30 * time.Second,
	}
}
