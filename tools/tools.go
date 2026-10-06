// Package tools implements the three tools the agent can call.
// Plan and provider data comes from a mock JSON file.
package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrEmployeeNotFound     = errors.New("employee not found")
	ErrUnsupportedSpecialty = errors.New("unsupported specialty")
	ErrEmptyReason          = errors.New("ticket reason is required")
)

// Specialties is the full list of supported specialties.
var Specialties = []string{
	"family_counseling",
	"marriage_counseling",
	"anxiety",
	"depression",
	"substance_use",
	"child_therapy",
}

// Plan is an employee's benefit coverage, keyed by specialty.
type Plan struct {
	PlanName string          `json:"plan_name"`
	Coverage map[string]bool `json:"coverage"`
}

type Provider struct {
	Name      string `json:"name"`
	ZipCode   string `json:"zip_code"`
	Specialty string `json:"specialty"`
	Phone     string `json:"phone"`
}

type Ticket struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Store holds the mock data and the tickets created in this run.
type Store struct {
	Plans     map[string]Plan `json:"plans"`
	Providers []Provider      `json:"providers"`

	tickets []Ticket
}

// Load reads the mock data file.
func Load(path string) (*Store, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mock data: %w", err)
	}
	var s Store
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, fmt.Errorf("parse mock data: %w", err)
	}
	if err := s.validate(); err != nil {
		return nil, fmt.Errorf("invalid mock data: %w", err)
	}
	return &s, nil
}

// validate rejects data that would silently give wrong answers:
// a plan missing a specialty reads as "not covered", and a provider
// with a misspelled specialty can never be found.
func (s *Store) validate() error {
	if len(s.Plans) == 0 {
		return errors.New("no plans")
	}
	for id, plan := range s.Plans {
		for _, sp := range Specialties {
			if _, ok := plan.Coverage[sp]; !ok {
				return fmt.Errorf("plan %s: missing coverage for %q", id, sp)
			}
		}
	}
	for _, p := range s.Providers {
		if !isSupported(p.Specialty) {
			return fmt.Errorf("provider %s: %w: %q", p.Name, ErrUnsupportedSpecialty, p.Specialty)
		}
	}
	return nil
}

// GetPlanDetails returns the coverage for one employee.
// The ID is matched case-insensitively ("e123" finds "E123").
func (s *Store) GetPlanDetails(employeeID string) (Plan, error) {
	plan, ok := s.Plans[strings.ToUpper(strings.TrimSpace(employeeID))]
	if !ok {
		return Plan{}, fmt.Errorf("%w: %q", ErrEmployeeNotFound, employeeID)
	}
	return plan, nil
}

// SearchProviders returns providers matching both zip code and specialty.
// No match is not an error; it returns an empty list.
func (s *Store) SearchProviders(zipCode, specialty string) ([]Provider, error) {
	zipCode = strings.TrimSpace(zipCode)
	specialty = strings.ToLower(strings.TrimSpace(specialty))
	if !isSupported(specialty) {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedSpecialty, specialty)
	}

	matches := []Provider{}
	for _, p := range s.Providers {
		if p.ZipCode == zipCode && p.Specialty == specialty {
			matches = append(matches, p)
		}
	}
	return matches, nil
}

// CreateSupportTicket records an escalation and returns its ticket.
func (s *Store) CreateSupportTicket(reason string) (Ticket, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Ticket{}, ErrEmptyReason
	}
	t := Ticket{
		ID:     fmt.Sprintf("TICKET-%03d", len(s.tickets)+1),
		Reason: reason,
	}
	s.tickets = append(s.tickets, t)
	return t, nil
}

func isSupported(specialty string) bool {
	for _, sp := range Specialties {
		if sp == specialty {
			return true
		}
	}
	return false
}
