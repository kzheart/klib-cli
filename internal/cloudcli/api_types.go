package cloudcli

import "time"

// These wire DTOs belong to the public HTTP client, not the server domain model.
type deploymentResponse struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Revision int64  `json:"revision"`
	Products []struct {
		ProductID string `json:"product_id"`
	} `json:"products"`
}
type enrollmentResponse struct {
	DeploymentID string    `json:"deployment_id"`
	Revision     int64     `json:"revision"`
	ExpiresAt    time.Time `json:"expires_at"`
	Token        string    `json:"token"`
}
