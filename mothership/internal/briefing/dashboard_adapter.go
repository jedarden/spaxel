// Package briefing provides dashboard adapter for morning briefing.
package briefing

import (
	"log"
)

// DashboardAdapter adapts the Generator to the dashboard BriefingProvider interface.
type DashboardAdapter struct {
	generator *Generator
}

// NewDashboardAdapter creates a new dashboard adapter.
func NewDashboardAdapter(gen *Generator) *DashboardAdapter {
	return &DashboardAdapter{generator: gen}
}

// GetTodayBriefing returns today's briefing as a map for the dashboard.
func (a *DashboardAdapter) GetTodayBriefing() (map[string]interface{}, error) {
	return a.generator.GetTodayBriefing()
}

// MarkDelivered marks a briefing as delivered.
func (a *DashboardAdapter) MarkDelivered(id string) error {
	return a.generator.MarkDelivered(id)
}

// ShouldPushBriefing checks if the briefing should be pushed to clients.
func (a *DashboardAdapter) ShouldPushBriefing() bool {
	return a.generator.ShouldPushBriefing()
}

// SetQuietHours sets the quiet hours range for overnight events.
func (a *DashboardAdapter) SetQuietHours(start, end int) {
	a.generator.SetQuietHours(start, end)
}

// SetWeatherAPIURL sets the weather API URL for weather section.
func (a *DashboardAdapter) SetWeatherAPIURL(url string) {
	a.generator.SetWeatherAPIURL(url)
}

// SetProviders sets the provider interfaces for briefing generation.
func (a *DashboardAdapter) SetProviders(z ZoneProvider, p PersonProvider, pr PredictionProvider, hp HealthProvider) {
	a.generator.SetProviders(z, p, pr, hp)
}

// SetNodeInfoProvider sets the node info provider.
func (a *DashboardAdapter) SetNodeInfoProvider(n NodeInfoProvider) {
	a.generator.SetNodeInfoProvider(n)
}

// Close closes the underlying generator.
func (a *DashboardAdapter) Close() error {
	return a.generator.Close()
}

// GetGenerator returns the underlying generator for direct access.
func (a *DashboardAdapter) GetGenerator() *Generator {
	return a.generator
}

// Generate creates a morning briefing for the given date and person.
func (a *DashboardAdapter) Generate(date, person string) (*Briefing, error) {
	return a.generator.Generate(date, person)
}

// Save persists a briefing to the database.
func (a *DashboardAdapter) Save(b *Briefing) error {
	return a.generator.Save(b)
}

// Get retrieves a previously generated briefing by date and optional person.
func (a *DashboardAdapter) Get(date, person string) (*Briefing, error) {
	return a.generator.Get(date, person)
}

// GetLatest retrieves the most recent briefing.
func (a *DashboardAdapter) GetLatest() (*Briefing, error) {
	return a.generator.GetLatest()
}

// ShouldGenerate checks if a briefing should be generated for the given date.
func (a *DashboardAdapter) ShouldGenerate(date, person string) bool {
	return a.generator.ShouldGenerate(date, person)
}

// MarkAcknowledged marks a briefing as acknowledged by the user.
func (a *DashboardAdapter) MarkAcknowledged(id string) error {
	return a.generator.MarkAcknowledged(id)
}

// GetBriefingForAPI returns a briefing formatted for API response with all fields.
func (a *DashboardAdapter) GetBriefingForAPI(date string, person string) (*Briefing, error) {
	b, err := a.generator.Get(date, person)
	if err != nil {
		// Try generating if not found
		b, err = a.generator.Generate(date, person)
		if err != nil {
			log.Printf("[ERROR] Failed to generate briefing for %s: %v", date, err)
			return nil, err
		}
		// Save the new briefing
		if err := a.generator.Save(b); err != nil {
			log.Printf("[ERROR] Failed to save briefing for %s: %v", date, err)
		}
	}
	return b, nil
}
