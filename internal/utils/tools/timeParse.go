package tools

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrValueEmpty = errors.New("value is required")
	ErrTimeUnit   = errors.New("unsupported time unit")
)

// Convert string with units to duration， Up to 9999
func StringToDuration(value string) (time.Duration, error) {

	if value == "" {
		return time.Duration(0), ErrValueEmpty
	}

	pattern := regexp.MustCompile("^[1-9]{1}[0-9]{0,3}[DdHhMmSs]$")
	if !pattern.MatchString(value) {
		return time.Duration(0), ErrValueEmpty
	}

	str := pattern.FindAllString(value, 1)
	if len(str) == 0 {
		return time.Duration(0), ErrValueEmpty
	}
	value = str[0]
	unit := value[len(value)-1]
	t := strings.TrimSuffix(value, string(unit))
	term, err := strconv.Atoi(t)
	if err != nil {
		return time.Duration(0), ErrValueEmpty
	}
	switch unit {
	case 'd', 'D':

		return time.Hour * 24 * time.Duration(term), nil
	case 'h', 'H':
		return time.Hour * time.Duration(term), nil

	case 'm', 'M':
		return time.Minute * time.Duration(term), nil

	case 's', 'S':
		return time.Second * time.Duration(term), nil

	}
	return time.Duration(0), ErrTimeUnit
}
