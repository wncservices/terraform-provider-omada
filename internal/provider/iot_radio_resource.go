// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/resource"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

// omada_iot_radio manages the IoT radio on the access points
// (Settings -> IoT -> Radio Settings).
//
// This is the first settings document served **only** by the Open API — the web
// API answers -1600 for the same path — so unlike omada_switch_port, where only
// the write crosses over, this one needs Open API credentials to refresh too.
func NewIoTRadioResource() resource.Resource {
	return newSettingsResource(settingsSpec{
		typeName: "iot_radio",
		doc:      omada.IoTRadioSetting,
		desc: "Manages the IoT (Zigbee/BLE) radio on the access points.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **This resource needs Open API credentials** (`openapi_client_id` / " +
			"`openapi_client_secret`). This settings document is served only by the Open API; the web " +
			"API answers `-1600` for the same path. Unlike `omada_switch_port`, where only the write " +
			"goes through the Open API, here **reading needs them as well** — so a refresh fails " +
			"without them, not just an apply.",
		fields: []settingField{
			{"enable", "enable", kindBool, "Whether the IoT radio is on."},
			{"console_mode", "consoleMode", kindInt,
				"Controller enum for the console mode. `0` is the value observed on live hardware; " +
					"the controller does not publish the rest."},
			{"passcode", "passcode", kindStringWO,
				"Passcode for pairing IoT devices. **Write-only** — it is sent to the controller but " +
					"never stored in Terraform state, so it cannot leak through a state file or a " +
					"remote backend. Because it is never read back, a change made outside Terraform " +
					"will not show as drift."},
			{"transmit_power", "transmitPower", kindInt,
				"Controller enum for radio transmit power. Lower power means a smaller radio " +
					"footprint, which for a protocol as weakly authenticated as BLE pairing is a " +
					"meaningful part of the security boundary — prefer the lowest setting that covers " +
					"the devices you actually have."},
			{"aging_time", "agingTime", kindInt, "How long a discovered IoT device is remembered, in minutes."},
			{"format", "format", kindInt, "Controller enum for the advertised data format."},
		},
	})
}
