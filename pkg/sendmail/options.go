package sendmail

type options struct {
	saveToSentItems bool
}

type Option func(*options)

// WithSaveToSentItems sets whether the sent Message is saved to the Sent Items
// folder of the sending mailbox.
func WithSaveToSentItems(save bool) Option {
	return func(o *options) {
		o.saveToSentItems = save
	}
}
