package airtable

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WebhookUser represents a user associated with a webhook action.
type WebhookUser struct {
	Id              string `json:"id"`
	Email           string `json:"email"`
	PermissionLevel string `json:"permissionLevel"`
}

// ActionMetadata represents metadata about the webhook action.
type ActionMetadata struct {
	Source         string `json:"source"`
	SourceMetadata struct {
		User WebhookUser `json:"user"`
	} `json:"sourceMetadata"`
}

// WebhookPayload represents the common structure of a webhook payload.
type WebhookPayload struct {
	Timestamp             time.Time      `json:"timestamp"`
	BaseTransactionNumber int            `json:"baseTransactionNumber"`
	ActionMetadata        ActionMetadata `json:"actionMetadata"`
	PayloadFormat         string         `json:"payloadFormat"`
	ChangedTablesById     any            `json:"changedTablesById,omitempty"`
	CreatedTablesById     any            `json:"createdTablesById,omitempty"`
	DestroyedTableIds     []string       `json:"destroyedTableIds,omitempty"`
	Error                 bool           `json:"error,omitempty"`
	Code                  string         `json:"code,omitempty"`
}

// WebhookResponse represents the response containing one or more payloads.
type WebhookResponse struct {
	Payloads      []WebhookPayload `json:"payloads"`
	Cursor        int              `json:"cursor"`
	MightHaveMore bool             `json:"mightHaveMore"`
}

// InitialWebhookResponse represents the structure of the initial payload received by the webhook.
type InitialWebhookResponse struct {
	Base struct {
		Id string `json:"id"`
	} `json:"base"`
	Webhook struct {
		Id string `json:"id"`
	} `json:"webhook"`
	Timestamp time.Time `json:"timestamp"`
}

// InitialWebhookResponseHandler is a struct that holds a channel for InitialWebhookResponse payloads.
type InitialWebhookResponseHandler struct {
	macSecret []byte

	C chan InitialWebhookResponse
}

func (h *InitialWebhookResponseHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read and store the body of the request
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("error reading request body: %v", err), http.StatusBadRequest)
		return
	}

	// Compute HMAC
	mac := hmac.New(sha256.New, h.macSecret)
	mac.Write(body)
	expectedHMAC := mac.Sum(nil)

	// Extract the HMAC from the request header
	recievedHMAC, err := hex.DecodeString(strings.TrimPrefix(r.Header.Get("X-Airtable-Content-MAC"), "hmac-sha256="))
	if err != nil {
		http.Error(w, "invalid HMAC in header", http.StatusBadRequest)
		return
	}

	// Compare the two HMACs
	if !hmac.Equal(recievedHMAC, expectedHMAC) {
		http.Error(w, "invalid HMAC", http.StatusUnauthorized)
		return
	}

	// Decode the payload
	var initialPayload InitialWebhookResponse
	if err := json.Unmarshal(body, &initialPayload); err != nil {
		http.Error(w, fmt.Sprintf("error decoding initial payload: %v", err), http.StatusBadRequest)
		return
	}

	// Send the initial payload to the channel
	select {
	case h.C <- initialPayload:
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "Initial webhook response received")
	default:
		http.Error(w, "channel is full, unable to process initial webhook response at this time", http.StatusServiceUnavailable)
	}
}

// NewInitialWebhookResponseHandler creates a new InitialWebhookResponseHandler with a channel for InitialWebhookPayloads.
func NewInitialWebhookResponseHandler(macSecretBase64 string) (*InitialWebhookResponseHandler, error) {
	// decode the secret
	macSecret, err := base64.StdEncoding.DecodeString(macSecretBase64)
	if err != nil {
		return nil, fmt.Errorf("error decoding mac secret: %v", err)
	}

	return &InitialWebhookResponseHandler{
		macSecret: macSecret,
		C:         make(chan InitialWebhookResponse, 1),
	}, nil
}

// WebhookManager is responsible for handling the entire webhook process.
type WebhookManager struct {
	initialResponseChan chan InitialWebhookResponse
	client              *Client
	lastCursor          int // Stores the last cursor value

	C chan WebhookPayload
}

// NewWebhookManager creates a new WebhookManager as a method of the Client type.
func (c *Client) NewWebhookManager(ch chan InitialWebhookResponse) *WebhookManager {
	return &WebhookManager{
		client:              c,
		initialResponseChan: ch,
		lastCursor:          1, // Initialize to 1 as per the documentation

		C: make(chan WebhookPayload, 1),
	}
}

// Run starts listening on the InitialResponseChan for incoming initial webhook responses.
func (h *WebhookManager) Run() error {
	for initialResponse := range h.initialResponseChan {
		err := h.FetchWebhookPayloads(initialResponse.Base.Id, initialResponse.Webhook.Id)
		if err != nil {
			return err // Return the error encountered
		}
	}
	return nil // Return nil if the loop exits normally
}

// FetchWebhookPayloads fetches the detailed WebhookPayloads from Airtable.
func (h *WebhookManager) FetchWebhookPayloads(baseId, webhookId string) error {
	for {
		path := []string{"bases", baseId, "webhooks", webhookId, "payloads"}
		params := []param{{key: "cursor", value: fmt.Sprintf("%d", h.lastCursor)}}

		data, err := h.client.get(path, params)
		if err != nil {
			return fmt.Errorf("error fetching webhook payloads: %v", err)
		}

		var webhookResponse WebhookResponse
		if err := json.Unmarshal(data, &webhookResponse); err != nil {
			return fmt.Errorf("error decoding response: %v", err)
		}

		// Process and send each payload in the response
		for _, payload := range webhookResponse.Payloads {
			h.C <- payload
		}

		// Update the last cursor with the new value
		h.lastCursor = webhookResponse.Cursor

		// Check if there are more payloads to fetch
		if !webhookResponse.MightHaveMore {
			break // Exit the loop if no more payloads are available
		}
	}

	return nil
}

// WebhookHandler encapsulates InitialWebhookResponseHandler and WebhookManager.
type WebhookHandler struct {
	initialHandler *InitialWebhookResponseHandler
	manager        *WebhookManager

	C chan WebhookPayload
}

// NewWebhookHandler creates a new WebhookHandler as a method of the Client type.
func (c *Client) NewWebhookHandler(macSecretBase64 string) (*WebhookHandler, error) {
	initialHandler, err := NewInitialWebhookResponseHandler(macSecretBase64)
	if err != nil {
		return nil, fmt.Errorf("error creating initial webhook response handler: %v", err)
	}

	manager := c.NewWebhookManager(initialHandler.C)

	return &WebhookHandler{
		initialHandler: initialHandler,
		manager:        manager,
		C:              manager.C,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.initialHandler.ServeHTTP(w, r) // Delegate to InitialWebhookResponseHandler
}

// Run starts the process of listening for and fetching webhook payloads.
func (h *WebhookHandler) Run() error {
	return h.manager.Run() // Delegate to WebhookManager's Run method
}

//
