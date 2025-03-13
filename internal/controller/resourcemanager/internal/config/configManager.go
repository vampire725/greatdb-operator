package config

import (
	"fmt"
	"runtime"

	ini "gopkg.in/ini.v1"
)

var opts = ini.LoadOptions{
	// Do not error on nonexistent file to allow empty string as filename input
	Loose: true,
	// MySQL ini file can have boolean keys.
	AllowBooleanKeys: true,
	// Ignore ; in "plugin-load"
	IgnoreInlineComment: true,
	// MySQL ini file can have keys with same name under same section.
	AllowShadows: true,
}

// IniParser
type IniParser struct {
	configReader *ini.File
}

func init() {

	if runtime.GOOS == "windows" {
		ini.LineBreak = "\n"
	}

}

// NewIniParser Create a new ini file parser
func NewIniParserforFile(fileName string) (*IniParser, error) {
	conf, err := ini.LoadSources(opts, fileName)
	if err != nil {
		return nil, err
	}

	return &IniParser{
		configReader: conf,
	}, nil
}

func NewIniParserforByte(data []byte) (*IniParser, error) {
	conf, err := ini.LoadSources(opts, data)
	if err != nil {
		return nil, err
	}
	return &IniParser{
		configReader: conf,
	}, nil
}

// GetString Get a value and convert it to a string
func (i *IniParser) GetValueString(section, key string) string {

	s := i.configReader.Section(section)

	if s == nil {
		return ""
	}
	return s.Key(key).String()
}

// GetValue Get a Key
func (i *IniParser) GetKey(section, key string) (*ini.Key, error) {

	s := i.configReader.Section(section)

	if s == nil {
		return nil, fmt.Errorf("section %s not exist", section)
	}

	if !s.HasKey(key) {
		return nil, fmt.Errorf("key %s not exist in the section %s", key, section)
	}
	return s.Key(key), nil
}

func (i *IniParser) SetValue(section, key, value string) error {

	s := i.configReader.Section(section)
	if s == nil {
		return fmt.Errorf("section %s not exist", section)
	}

	s.Key(key).SetValue(value)
	return nil
}

func (i *IniParser) BatchSet(section string, data map[string]string) error {

	s := i.configReader.Section(section)
	if s == nil {
		return fmt.Errorf("section %s not exist", section)
	}

	for key, value := range data {
		s.Key(key).SetValue(value)
	}

	return nil
}

// AddKeyValue The key cannot be added unless it does not exist
func (i *IniParser) AddKeyValue(section, key, value string) error {
	s := i.configReader.Section(section)
	if s == nil {
		return fmt.Errorf("section %s not exist", section)
	}

	if s.HasKey(key) {
		return fmt.Errorf("key %s  already exists in the section %s", key, section)
	}

	s.Key(key).SetValue(value)
	return nil

}

func (i *IniParser) KeyIsexist(section, key string) (bool, error) {
	s := i.configReader.Section(section)
	if s == nil {
		return false, fmt.Errorf("section %s not exist", section)
	}

	return s.HasKey(key), nil
}

func (i *IniParser) SaveToString() string {
	if i.configReader == nil {
		return ""
	}
	wr := &WriteData{}
	i.configReader.WriteTo(wr)
	return wr.data

}

func (i *IniParser) GetSection(section string) *ini.Section {
	return i.configReader.Section(section)
}
