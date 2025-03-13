package config

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

type configTemplate struct {
	Tmpl *template.Template
}

func NewconfigTemplate() *configTemplate {
	return &configTemplate{}
}

func (ct *configTemplate) LoadStringTemplate(templateName, data string) error {
	tmpl, err := template.New(templateName).Parse(data)
	if err != nil {
		return err
	}
	ct.Tmpl = tmpl
	return nil
}

func (ct *configTemplate) LoadFileTemplate(filename string) error {
	tmpl, err := template.ParseFiles(filename)
	if err != nil {
		return err
	}
	ct.Tmpl = tmpl
	return nil

}

func (ct *configTemplate) ExecToString(data interface{}) (string, error) {
	builder := &strings.Builder{}
	err := ct.Tmpl.Execute(builder, data)
	if err != nil {
		return "", err
	}
	return builder.String(), nil
}

func (ct *configTemplate) ExecToStdout(data interface{}) {

	err := ct.Tmpl.Execute(os.Stdout, data)

	if err != nil {
		fmt.Println(err)
	}
}
