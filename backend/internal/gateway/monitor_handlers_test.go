package gateway

import "testing"

func TestValidateMonitorSignalRequestIncidentSignals(t *testing.T) {
	tests := []struct {
		name    string
		req     createSignalRequest
		wantErr bool
	}{
		{
			name: "ack alert",
			req: createSignalRequest{
				SignalType: "ack_alert",
				TargetType: "alert",
				TargetID:   "alert:42",
				Reason:     "host acknowledged critical alert",
			},
		},
		{
			name: "mute alert",
			req: createSignalRequest{
				SignalType: "mute_alert_10m",
				TargetType: "alert",
				TargetID:   "alert:42",
				Reason:     "duplicate notification during active triage",
			},
		},
		{
			name: "merchant note on auction",
			req: createSignalRequest{
				SignalType: "merchant_incident_note",
				TargetType: "auction",
				TargetID:   "auc_live",
				Reason:     "customer support reported frozen bidder UI",
			},
		},
		{
			name: "merchant note on room",
			req: createSignalRequest{
				SignalType: "merchant_incident_note",
				TargetType: "room",
				TargetID:   "room_main",
				Reason:     "host observed delayed comments",
			},
		},
		{
			name: "ack requires alert target",
			req: createSignalRequest{
				SignalType: "ack_alert",
				TargetType: "auction",
				TargetID:   "auc_live",
				Reason:     "wrong target",
			},
			wantErr: true,
		},
		{
			name: "merchant note rejects arbitrary target",
			req: createSignalRequest{
				SignalType: "merchant_incident_note",
				TargetType: "user",
				TargetID:   "user_1",
				Reason:     "wrong target",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateMonitorSignalRequest(tt.req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateMonitorSignalRequest() err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}
