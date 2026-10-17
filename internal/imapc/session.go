package imapc

import (
	"time"
)

// Session keeps a Client alive across a long job and hides connection drops:
// every call retries after re-dialling, logging in again and re-selecting the
// mailbox that was open. A tagged NO or BAD from the server is a decision,
// not a glitch, so it is returned immediately and stops the job.
type Session struct {
	cfg     Config
	client  *Client
	mailbox string // raw name of the SELECTed mailbox, restored on reconnect
}

// Open dials, logs in and returns a ready session.
func Open(cfg Config) (*Session, error) {
	cfg.applyDefaults()
	c, err := Dial(cfg)
	if err != nil {
		return nil, err
	}
	return &Session{cfg: cfg, client: c}, nil
}

// Close ends the session.
func (s *Session) Close() {
	if s.client != nil {
		s.client.Logout()
		s.client = nil
	}
}

// Has reports a server capability of the live connection.
func (s *Session) Has(name string) bool {
	return s.client != nil && s.client.Has(name)
}

func (s *Session) reconnect() error {
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	c, err := Dial(s.cfg)
	if err != nil {
		return err
	}
	if s.mailbox != "" {
		if _, err := c.Select(s.mailbox); err != nil {
			c.Close()
			return err
		}
	}
	s.client = c
	return nil
}

// run performs one operation, retrying transport failures.
func (s *Session) run(fn func(c *Client) error) error {
	var lastErr error
	for attempt := 0; ; attempt++ {
		if s.client == nil {
			if err := s.reconnect(); err != nil {
				lastErr = err
				if IsServerError(err) || attempt >= s.cfg.Retries {
					return lastErr
				}
				time.Sleep(retryDelay)
				continue
			}
		}

		err := fn(s.client)
		if err == nil {
			return nil
		}
		if IsServerError(err) {
			return err
		}

		// The connection broke. Drop it and let the next round re-dial.
		lastErr = err
		s.client.Close()
		s.client = nil
		if attempt >= s.cfg.Retries {
			return lastErr
		}
		time.Sleep(retryDelay)
	}
}

// Folders lists every mailbox, names already decoded to UTF-8.
func (s *Session) Folders() ([]Folder, error) {
	var out []Folder
	err := s.run(func(c *Client) error {
		f, err := c.List()
		if err != nil {
			return err
		}
		out = f
		return nil
	})
	return out, err
}

// Select opens a mailbox and remembers it, so a reconnect lands back in the
// same place. raw must be a modified UTF-7 name.
func (s *Session) Select(raw string) (int, error) {
	total := 0
	err := s.run(func(c *Client) error {
		n, err := c.Select(raw)
		if err != nil {
			return err
		}
		total = n
		return nil
	})
	if err == nil {
		s.mailbox = raw
	}
	return total, err
}

// SearchUIDs lists the UIDs in the selected mailbox within the date range.
func (s *Session) SearchUIDs(since, until *time.Time) ([]uint32, error) {
	var uids []uint32
	err := s.run(func(c *Client) error {
		u, err := c.SearchUIDs(since, until)
		if err != nil {
			return err
		}
		uids = u
		return nil
	})
	return uids, err
}

// Fetch downloads one message.
func (s *Session) Fetch(uid uint32) (Message, error) {
	var msg Message
	err := s.run(func(c *Client) error {
		m, err := c.Fetch(uid)
		if err != nil {
			return err
		}
		msg = m
		return nil
	})
	return msg, err
}

// Move performs the copy/flag/expunge sequence for one batch of UIDs. The
// three steps run on the same connection; if the link drops midway the whole
// batch is retried, which at worst leaves a duplicate in the target folder
// rather than losing a message.
func (s *Session) Move(uids []uint32, destRaw string) error {
	if len(uids) == 0 {
		return nil
	}
	return s.run(func(c *Client) error {
		if err := c.Copy(uids, destRaw); err != nil {
			return err
		}
		if err := c.MarkDeleted(uids); err != nil {
			return err
		}
		return c.Expunge(uids)
	})
}
