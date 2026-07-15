package sendmail

import (
	"context"
	"net/mail"
	"strings"

	graphmodels "github.com/microsoftgraph/msgraph-sdk-go/models"
	graphusers "github.com/microsoftgraph/msgraph-sdk-go/users"
)

type Message struct {
	body        *graphmodels.ItemBody
	message     *graphmodels.Message
	requestBody *graphusers.ItemSendMailPostRequestBody
}

func NewMessage(from, to, subject string, opts ...MessageOption) *Message {
	m := &Message{
		body:        graphmodels.NewItemBody(),
		message:     graphmodels.NewMessage(),
		requestBody: graphusers.NewItemSendMailPostRequestBody(),
	}

	// apply options
	for _, o := range opts {
		o(m)
	}

	// set subject and message body
	m.message.SetSubject(&subject)
	m.message.SetBody(m.body)

	// add sender/from
	recipient := graphmodels.NewRecipient()
	emailAddress := graphmodels.NewEmailAddress()
	emailAddress.SetAddress(&from)
	recipient.SetEmailAddress(emailAddress)
	m.message.SetFrom(recipient)

	// set recipients
	if addrs := parseAddressList(to); len(addrs) > 0 {
		m.message.SetToRecipients(addrs)
	}

	return m
}

func (m *Message) Send(ctx context.Context, user *graphusers.UserItemRequestBuilder) error {
	// create SendMailPostRequestBody
	m.requestBody.SetMessage(m.message)

	return user.SendMail().Post(ctx, m.requestBody, nil)
}

func parseAddressList(addresses string) []graphmodels.Recipientable {
	recipientList := []graphmodels.Recipientable{}

	if addresses == "" {
		return recipientList
	}

	// Split the address list by commas, trim spaces and parse as valid email address
	list := strings.Split(addresses, ",")
	for i := range list {
		a, err := mail.ParseAddress(strings.TrimSpace(list[i]))
		if err != nil {
			continue
		}
		address := a.Address

		// build recipient
		recipient := graphmodels.NewRecipient()
		emailAddress := graphmodels.NewEmailAddress()
		emailAddress.SetAddress(&address)
		recipient.SetEmailAddress(emailAddress)

		// add to list
		recipientList = append(recipientList, recipient)
	}

	return recipientList
}
