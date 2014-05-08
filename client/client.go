package client

import (
	"bufio"
	"bytes"
	"net/http"
)

type Client struct {
	Stream    <-chan Event
	Err       error
	closeChan chan struct{}
}

type Event struct {
	Id   string
	Type string
	Data []byte
}

func New(url string) (*Client, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Close = true
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	stream := make(chan Event, 1024)
	client := &Client{
		Stream: stream,
	}
	go func() {
		defer close(stream)

		go func() {
			<-client.closeChan
			resp.Body.Close()
			for _ = range client.closeChan {
			}
		}()

		s := bufio.NewScanner(resp.Body)
		s.Split(scanLines)

		var event Event
		for s.Scan() {
			line := s.Bytes()
			if len(line) == 0 {
				stream <- event
				event = Event{}
			}
			field := line
			value := []byte{}
			if colon := bytes.IndexByte(line, ':'); colon != -1 {
				if colon == 0 {
					continue // comment
				}
				field = line[:colon]
				value = line[colon+1:]
				if value[0] == ' ' {
					value = value[1:]
				}
			}
			switch string(field) {
			case "event":
				event.Type = string(value)
			case "data":
				event.Data = append(append(event.Data, value...), '\n')
			case "id":
				event.Id = string(value)
			case "retry":
				// TODO
			default:
				// ignored
			}
		}
		client.Err = s.Err()
		resp.Body.Close()
	}()
	return client, nil
}

func (c *Client) Close() error {
	c.closeChan <- struct{}{}
	return nil
}

func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		if data[i] == '\r' {
			if i == len(data)-1 {
				if atEOF {
					// final line
					return len(data), data[:len(data)-1], nil
				}
				return 0, nil, nil // LF may follow, request more data
			}
			if data[i+1] == '\n' {
				return i + 2, data[:i], nil
			}
			return i + 1, data[:i], nil
		}
		// data[i] == '\n'
		return i + 1, data[:i], nil
	}
	if atEOF {
		// final line
		return len(data), data, nil
	}
	// request more data
	return 0, nil, nil
}
