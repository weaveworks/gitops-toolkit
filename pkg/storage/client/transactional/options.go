package transactional

import "time"

type TxOption interface {
	ApplyToTx(*TxOptions)
}

var _ TxOption = &TxOptions{}

func defaultTxOptions() *TxOptions {
	return &TxOptions{
		Timeout: 1 * time.Minute,
		Mode:    TxModeAtomic,
	}
}

type TxOptions struct {
	Timeout time.Duration
	Mode    TxMode
}

func (o *TxOptions) ApplyToTx(target *TxOptions) {
	if o.Timeout != 0 {
		target.Timeout = o.Timeout
	}
	if len(o.Mode) != 0 {
		target.Mode = o.Mode
	}
}

func (o *TxOptions) ApplyOptions(opts []TxOption) *TxOptions {
	for _, opt := range opts {
		opt.ApplyToTx(o)
	}
	return o
}

var _ TxOption = TxMode("")

type TxMode string

const (
	// TxModeAtomic makes the transaction fully atomic, i.e. so
	// that any read happening against the target branch during the
	// lifetime of the transaction will be blocked until the completition
	// of the transaction.
	TxModeAtomic TxMode = "Atomic"
	// TxModeAllowReading will allow reads targeting the given
	// branch a transaction is executing against; but before the
	// transaction has completed all reads will strictly return
	// the data available prior to the transaction taking place.
	TxModeAllowReading TxMode = "AllowReading"
)

func (m TxMode) ApplyToTx(target *TxOptions) {
	target.Mode = m
}

var _ TxOption = TxTimeout(0)

type TxTimeout time.Duration

func (t TxTimeout) ApplyToTx(target *TxOptions) {
	target.Timeout = time.Duration(t)
}
