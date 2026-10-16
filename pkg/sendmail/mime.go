package sendmail

import (
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
)

// parsedMessage is the content of a MIME message that becomes a Message.
type parsedMessage struct {
	header      message.Header
	subject     string
	textBody    string
	htmlBody    string
	attachments []attachment
}

// attachment is a file from a MIME message. An inline attachment is one an
// HTML body references by its content ID.
type attachment struct {
	name        string
	contentType string
	contentID   string
	inline      bool
	data        []byte
}

// parseMIME reads a MIME message, decoding transfer encodings and converting
// text to UTF-8.
func parseMIME(r io.Reader) (*parsedMessage, error) {
	e, err := message.Read(r)
	if err != nil && !isUnknownEncoding(err) {
		return nil, err
	}

	p := &parsedMessage{header: e.Header}

	// on error Subject returns the raw header value
	p.subject, _ = (&gomail.Header{Header: e.Header}).Subject()

	err = e.Walk(func(path []int, part *message.Entity, err error) error {
		// a part with an unknown encoding or charset is kept as is
		if err != nil && !isUnknownEncoding(err) {
			return err
		}

		if part.MultipartReader() != nil {
			return nil
		}

		data, err := io.ReadAll(part.Body)
		if err != nil {
			return fmt.Errorf("could not read message part: %w", err)
		}

		mediaType, _, _ := part.Header.ContentType()
		disposition, _, _ := part.Header.ContentDisposition()
		filename, _ := (&gomail.AttachmentHeader{Header: part.Header}).Filename()
		contentID := strings.Trim(part.Header.Get("Content-Id"), "<>")
		isBody := disposition != "attachment" && filename == ""

		switch {
		case isBody && mediaType == "text/html" && p.htmlBody == "":
			p.htmlBody = trimNewline(string(data))
		case isBody && (mediaType == "text/plain" || mediaType == "") && p.textBody == "":
			p.textBody = trimNewline(string(data))
		case contentID != "" && disposition != "attachment":
			if filename == "" {
				filename = contentID
			}
			p.attachments = append(p.attachments, attachment{
				name:        filename,
				contentType: mediaType,
				contentID:   contentID,
				inline:      true,
				data:        data,
			})
		default:
			if filename == "" {
				filename = "attachment"
			}
			p.attachments = append(p.attachments, attachment{
				name:        filename,
				contentType: mediaType,
				data:        data,
			})
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return p, nil
}

func isUnknownEncoding(err error) bool {
	return message.IsUnknownCharset(err) || message.IsUnknownEncoding(err)
}

// trimNewline removes the line ending that terminates a body part.
func trimNewline(s string) string {
	s = strings.TrimSuffix(s, "\n")

	return strings.TrimSuffix(s, "\r")
}
