package model

import "time"

type SourceInfo struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Commit      string `json:"commit"`
	SourceID    string `json:"sourceId"`
	LicenseNote string `json:"licenseNote"`
}

type PoemPayload struct {
	ID           string     `json:"id"`
	Title        string     `json:"title"`
	Author       string     `json:"author"`
	Dynasty      string     `json:"dynasty"`
	Kind         string     `json:"kind"`
	Form         string     `json:"form"`
	Cipai        string     `json:"cipai,omitempty"`
	Lines        []string   `json:"lines"`
	Pinyin       []string   `json:"pinyin"`
	Translation  string     `json:"translation"`
	Annotations  []string   `json:"annotations"`
	Appreciation string     `json:"appreciation,omitempty"`
	Themes       []string   `json:"themes"`
	Collections  []string   `json:"collections"`
	AgeMin       int        `json:"ageMin"`
	AgeMax       int        `json:"ageMax"`
	PopularScore int        `json:"popularScore"`
	ContentHash  string     `json:"contentHash"`
	Source       SourceInfo `json:"source"`
}

type PoemListItem struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Author         string   `json:"author"`
	Dynasty        string   `json:"dynasty"`
	Kind           string   `json:"kind"`
	Form           string   `json:"form"`
	Cipai          string   `json:"cipai,omitempty"`
	Excerpt        string   `json:"excerpt"`
	Themes         []string `json:"themes"`
	Collections    []string `json:"collections"`
	AgeMin         int      `json:"ageMin"`
	AgeMax         int      `json:"ageMax"`
	HasPinyin      bool     `json:"hasPinyin"`
	HasTranslation bool     `json:"hasTranslation"`
	HasAnnotations bool     `json:"hasAnnotations"`
	PopularScore   int      `json:"popularScore"`
}

type DatasetManifest struct {
	Version     string            `json:"version"`
	GeneratedAt time.Time         `json:"generatedAt"`
	Count       int               `json:"count"`
	SHA256      string            `json:"sha256"`
	Sources     map[string]string `json:"sources"`
	Selection   string            `json:"selection"`
	Quality     map[string]int    `json:"quality"`
}
