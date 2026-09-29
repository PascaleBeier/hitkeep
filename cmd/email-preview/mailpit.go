package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	json "hitkeep/jsonapi"
	"hitkeep/mailer"
)

// mailpit is a minimal client for the Mailpit v1 HTTP API.
type mailpit struct {
	baseURL string
}

var mailpitHTTP = &http.Client{Timeout: 30 * time.Second}

type mailpitAddress struct {
	Email string `json:"Email"`
	Name  string `json:"Name,omitempty"`
}

type mailpitAttachment struct {
	Content     string `json:"Content"`
	Filename    string `json:"Filename"`
	ContentType string `json:"ContentType"`
	ContentID   string `json:"ContentID"`
}

type mailpitSendRequest struct {
	From    mailpitAddress   `json:"From"`
	To      []mailpitAddress `json:"To"`
	Subject string           `json:"Subject"`
	HTML    string           `json:"HTML"`
	// Attachments carry inline images so Mailpit sees the cid: HTML that
	// SMTP delivers, not a preview rewrite.
	Attachments []mailpitAttachment `json:"Attachments,omitempty"`
	Text        string              `json:"Text"`
	Tags        []string            `json:"Tags"`
}

type htmlCheckResult struct {
	Total struct {
		Supported float64 `json:"Supported"`
	} `json:"Total"`
	Warnings []struct {
		Slug  string `json:"Slug"`
		Title string `json:"Title"`
	} `json:"Warnings"`
}

func (m *mailpit) version() (string, error) {
	var info struct {
		Version string `json:"Version"`
	}
	return info.Version, m.do(http.MethodGet, "/api/v1/info", nil, &info)
}

func (m *mailpit) deleteTag(tag string) error {
	return m.do(http.MethodDelete, "/api/v1/search?query="+url.QueryEscape("tag:"+tag), nil, nil)
}

func (m *mailpit) send(rendered mailer.Rendered, tags []string) (string, error) {
	request := mailpitSendRequest{
		From:        mailpitAddress{Email: "noreply@hitkeep.example", Name: "HitKeep Preview"},
		To:          []mailpitAddress{{Email: "preview@hitkeep.example"}},
		Subject:     rendered.Subject,
		HTML:        rendered.HTML,
		Attachments: inlineAttachments(rendered.Inline),
		Text:        rendered.Text,
		Tags:        tags,
	}
	var response struct {
		ID string `json:"ID"`
	}
	return response.ID, m.do(http.MethodPost, "/api/v1/send", request, &response)
}

func inlineAttachments(images []mailer.InlineImage) []mailpitAttachment {
	attachments := make([]mailpitAttachment, 0, len(images))
	for _, image := range images {
		attachments = append(attachments, mailpitAttachment{
			Content:     base64.StdEncoding.EncodeToString(image.Data),
			Filename:    image.CID + ".png",
			ContentType: image.ContentType,
			ContentID:   image.CID,
		})
	}
	return attachments
}

func (m *mailpit) htmlCheck(id string) (htmlCheckResult, error) {
	var result htmlCheckResult
	return result, m.do(http.MethodGet, "/api/v1/message/"+url.PathEscape(id)+"/html-check", nil, &result)
}

func (m *mailpit) do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(context.Background(), method, m.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := mailpitHTTP.Do(request) // #nosec G704 -- operator-supplied local Mailpit URL
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return err
	}
	if response.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, response.StatusCode, bytes.TrimSpace(data))
	}
	if out == nil {
		return nil
	}
	// Mailpit responses carry more fields than this client reads.
	return json.Unmarshal(data, out)
}
