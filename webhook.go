package airtable

import (
	"encoding/json"
	"fmt"
	"time"
)

// filters defines filter options for the webhook.
type filters struct {
	RecordChangeScope     string   `json:"recordChangeScope,omitempty"`
	DataTypes             []string `json:"dataTypes"`
	ChangeTypes           []string `json:"changeTypes,omitempty"`
	FromSources           []string `json:"fromSources,omitempty"`
	SourceOptions         *any     `json:"sourceOptions,omitempty"`
	WatchDataInFieldIds   []string `json:"watchDataInFieldIds,omitempty"`
	WatchSchemaOfFieldIds []string `json:"watchSchemaOfFieldIds,omitempty"`
}

// WebhookSpecification defines the structure for the webhook creation request.
type WebhookSpecification struct {
	Options struct {
		Filters filters `json:"filters"`
	} `json:"options"`
}

func NewWebhookSpecification() *WebhookSpecification {
	return &WebhookSpecification{}
}

// SetRecordChangeScope sets the recordChangeScope.
func (w *WebhookSpecification) SetRecordChangeScope(scope string) *WebhookSpecification {
	w.Options.Filters.RecordChangeScope = scope
	return w
}

// AddDataType adds a dataType to the list.
func (w *WebhookSpecification) AddDataType(dataType string) *WebhookSpecification {
	w.Options.Filters.DataTypes = append(w.Options.Filters.DataTypes, dataType)
	return w
}

// AddChangeType adds a changeType to the list.
func (w *WebhookSpecification) AddChangeType(changeType string) *WebhookSpecification {
	w.Options.Filters.ChangeTypes = append(w.Options.Filters.ChangeTypes, changeType)
	return w
}

// AddFromSource adds a fromSource to the list.
func (w *WebhookSpecification) AddFromSource(source string) *WebhookSpecification {
	w.Options.Filters.FromSources = append(w.Options.Filters.FromSources, source)
	return w
}

// AddWatchDataInField adds a field ID to watch data in.
func (w *WebhookSpecification) AddWatchDataInField(fieldID string) *WebhookSpecification {
	w.Options.Filters.WatchDataInFieldIds = append(w.Options.Filters.WatchDataInFieldIds, fieldID)
	return w
}

// AddWatchSchemaOfField adds a field ID to watch schema changes in.
func (w *WebhookSpecification) AddWatchSchemaOfField(fieldID string) *WebhookSpecification {
	w.Options.Filters.WatchSchemaOfFieldIds = append(w.Options.Filters.WatchSchemaOfFieldIds, fieldID)
	return w
}

// CreateWebhookResponse defines the expected structure of the response from the Airtable API.
type CreateWebhookResponse struct {
	ID              string    `json:"id"`
	MacSecretBase64 string    `json:"macSecretBase64"`
	ExpirationTime  time.Time `json:"expirationTime"`
}

// CreateWebhook creates a new webhook in Airtable.
func (c *Client) CreateWebhook(baseId string, notificationUrl string, spec *WebhookSpecification) (*CreateWebhookResponse, error) {
	// Construct an anonymous struct for the payload
	payload := struct {
		NotificationUrl string                `json:"notificationUrl"`
		Specification   *WebhookSpecification `json:"specification,omitempty"`
	}{notificationUrl, spec}

	path := []string{"bases", baseId, "webhooks"}
	data, err := c.post(path, &payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook: %w", err)
	}

	var resp CreateWebhookResponse
	err = json.Unmarshal(data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal webhook response: %w", err)
	}

	return &resp, nil
}

// DeleteWebhook deletes an existing webhook in Airtable.
func (c *Client) DeleteWebhook(baseId, webhookId string) error {
	path := []string{"bases", baseId, "webhooks", webhookId}
	_, err := c.delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete webhook: %w", err)
	}

	return nil
}

// SetWebhookNotifications enables or disables notification pings for a webhook.
func (c *Client) SetWebhookNotifications(baseId, webhookId string, enable bool) error {
	path := []string{"bases", baseId, "webhooks", webhookId, "enableNotifications"}

	// Define the request payload struct directly within the function
	type enableNotificationPayload struct {
		Enable bool `json:"enable"`
	}

	// Create an instance of the payload struct with the provided value
	payload := enableNotificationPayload{
		Enable: enable,
	}

	// Use the payload in the POST request
	_, err := c.post(path, payload)
	if err != nil {
		return fmt.Errorf("failed to set webhook notifications: %w", err)
	}

	return nil
}

// RefreshWebhookResponse defines the expected structure of the response from refreshing a webhook.
type RefreshWebhookResponse struct {
	ExpirationTime time.Time `json:"expirationTime"`
}

// RefreshWebhook extends the life of a webhook. The new expiration time will be 7 days after the refresh time.
func (c *Client) RefreshWebhook(baseId, webhookId string) (*RefreshWebhookResponse, error) {
	path := []string{"bases", baseId, "webhooks", webhookId, "refresh"}

	data, err := c.post(path, nil) // No payload needed for this request
	if err != nil {
		return nil, fmt.Errorf("failed to refresh webhook: %w", err)
	}

	var resp RefreshWebhookResponse
	err = json.Unmarshal(data, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal refresh webhook response: %w", err)
	}

	return &resp, nil
}
