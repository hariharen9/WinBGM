package backend

import (
	"encoding/xml"
	"testing"
)

func TestBootProfileParser(t *testing.T) {
	sampleXML := `<Events>
		<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
			<System>
				<EventID>100</EventID>
				<TimeCreated SystemTime="2026-11-23T08:00:00.0000000Z"/>
			</System>
			<EventData>
				<Data Name="BootStartTime">2026-11-23T08:00:00.0000000Z</Data>
				<Data Name="BootEndTime">2026-11-23T08:00:26.4000000Z</Data>
				<Data Name="BootDuration">26400</Data>
				<Data Name="MainPathBootTime">18200</Data>
			</EventData>
		</Event>
		<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
			<System>
				<EventID>101</EventID>
				<TimeCreated SystemTime="2026-11-23T08:00:05.0000000Z"/>
			</System>
			<EventData>
				<Data Name="Name">Docker Desktop</Data>
				<Data Name="Path">C:\Program Files\Docker\Docker\Docker Desktop.exe</Data>
				<Data Name="TotalTime">4200</Data>
				<Data Name="DegradationTime">3800</Data>
			</EventData>
		</Event>
		<Event xmlns="http://schemas.microsoft.com/win/2004/08/events/event">
			<System>
				<EventID>103</EventID>
				<TimeCreated SystemTime="2026-11-23T08:00:07.0000000Z"/>
			</System>
			<EventData>
				<Data Name="Name">Spotify Free</Data>
				<Data Name="Path">C:\Users\Admin\AppData\Roaming\Spotify\Spotify.exe</Data>
				<Data Name="TotalTime">1800</Data>
				<Data Name="DegradationTime">1800</Data>
			</EventData>
		</Event>
	</Events>`

	var parsed xmlEvents
	if err := xml.Unmarshal([]byte(sampleXML), &parsed); err != nil {
		t.Fatalf("XML parse failed: %v", err)
	}

	if len(parsed.Events) != 3 {
		t.Fatalf("Expected 3 events, got %d", len(parsed.Events))
	}
	t.Logf("Parsed %d events successfully", len(parsed.Events))
}

func TestPowerReportParser(t *testing.T) {
	report := GetPowerReport()
	t.Logf("PowerReport: available=%v, requiresAdmin=%v, blockers=%d, summary=%q",
		report.Available, report.RequiresAdmin, len(report.Blockers), report.Summary)
}
