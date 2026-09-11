package response

type OaiDeviceflowResponse struct {
	DeviceAuthID    string `json:"device_auth_id"`
	UserCode        string `json:"user_code"`
	Interval        uint32 `json:"interval"`
	VerificationURL string `json:"verification_url"`
}

type OaiPostTokenResponse struct {
	Code         string `json:"authorization_code"`
	CodeVerifier string `json:"code_verifier"`
}
