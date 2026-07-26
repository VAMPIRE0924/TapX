package panel

import (
	"testing"

	"tapx/internal/model"
)

func TestPreservePanelAccessSettings(t *testing.T) {
	restored := []model.Settings{{
		ID:                     "global",
		Enabled:                false,
		PanelName:              "backup panel",
		PanelListen:            "0.0.0.0:9443",
		PanelDomain:            "backup.example",
		PanelBasePath:          "/backup/",
		PanelHTTPS:             false,
		PanelCertFile:          "/backup/fullchain.pem",
		PanelKeyFile:           "/backup/privkey.pem",
		PanelAuthEnabled:       false,
		AdminUsername:          "backup-admin",
		AdminPasswordHash:      "backup-hash",
		SessionTTLSecond:       900,
		ExternalXrayPath:       "/backup/xray",
		ExternalXrayConfigFile: "/backup/config.json",
	}}
	current := []model.Settings{{
		ID:                "global",
		Enabled:           true,
		PanelName:         "current panel",
		PanelListen:       "0.0.0.0:24443",
		PanelDomain:       "118.25.47.217",
		PanelBasePath:     "/tapx-lab/",
		PanelHTTPS:        true,
		PanelCertFile:     "/etc/letsencrypt/live/118.25.47.217/fullchain.pem",
		PanelKeyFile:      "/etc/letsencrypt/live/118.25.47.217/privkey.pem",
		PanelAuthEnabled:  true,
		AdminUsername:     "current-admin",
		AdminPasswordHash: "current-hash",
		SessionTTLSecond:  7200,
		ExternalXrayPath:  "/current/xray",
	}}

	got := preservePanelAccessSettings(restored, current)
	if len(got) != 1 {
		t.Fatalf("settings rows = %d, want one", len(got))
	}
	settings := got[0]
	if settings.Enabled != current[0].Enabled ||
		settings.PanelListen != current[0].PanelListen ||
		settings.PanelDomain != current[0].PanelDomain ||
		settings.PanelBasePath != current[0].PanelBasePath ||
		settings.PanelHTTPS != current[0].PanelHTTPS ||
		settings.PanelCertFile != current[0].PanelCertFile ||
		settings.PanelKeyFile != current[0].PanelKeyFile ||
		settings.PanelAuthEnabled != current[0].PanelAuthEnabled ||
		settings.AdminUsername != current[0].AdminUsername ||
		settings.AdminPasswordHash != current[0].AdminPasswordHash ||
		settings.SessionTTLSecond != current[0].SessionTTLSecond {
		t.Fatalf("machine-local panel access settings were not preserved: %+v", settings)
	}
	if settings.PanelName != restored[0].PanelName ||
		settings.ExternalXrayPath != restored[0].ExternalXrayPath ||
		settings.ExternalXrayConfigFile != restored[0].ExternalXrayConfigFile {
		t.Fatalf("portable workload settings were not restored: %+v", settings)
	}
}
