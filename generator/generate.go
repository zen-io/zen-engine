package generator

import (
	"fmt"
	"reflect"
	"strings"

	"golang.org/x/exp/slices"
)

func GenerateTarget[T any](name string, v T) string {
	gen := Generator{}
	gen.templateStructWithHeaders(name, v, 0)
	return gen.ToString()
}

func DescribeTarget[T any](v T) string {
	gen := Generator{}
	gen.describeStruct(v)
	return gen.ToString()
}

type Generator struct {
	sb strings.Builder
}

func (gen *Generator) ToString() string {
	return gen.sb.String()
}

func (gen *Generator) templateStructWithHeaders(name string, s interface{}, spaces int) {
	gen.sb.WriteString(fmt.Sprintf("%s {\n", name))

	gen.templateStruct(s, spaces+2)
	gen.sb.WriteString("}\n")
}

func (gen *Generator) templateStruct(s interface{}, spaces int) {
	t := reflect.TypeOf(s)
	v := reflect.ValueOf(s)

	if t.Kind() != reflect.Struct {
		return
	}

	spaceString := ""
	for k := 0; k < spaces; k++ {
		spaceString += " "
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		splConf := strings.Split(field.Tag.Get("mapstructure"), ",")
		fieldName := splConf[0]

		if !value.IsValid() || value.Kind() == reflect.Ptr {
			if field.Type.Elem().Kind() == reflect.Struct {
				if value.IsNil() {
					gen.sb.WriteString(fmt.Sprintf("%s%s = {}\n", spaceString, fieldName))
				} else if slices.Contains(splConf, "squash") {
					gen.templateStruct(value.Interface(), spaces)
				} else {
					gen.templateStructWithHeaders(fieldName, value.Interface(), spaces)
				}
			} else if value.IsNil() {
				gen.sb.WriteString(fmt.Sprintf("%s%s = null\n", spaceString, fieldName))
			} else if field.Type.Elem().Kind() == reflect.String {
				gen.sb.WriteString(fmt.Sprintf("%s%s = \"%v\"\n", spaceString, fieldName, value.Interface()))
			} else {
				gen.sb.WriteString(fmt.Sprintf("%s%s = %v\n", spaceString, fieldName, value.Interface()))
			}

			continue
		}

		if value.Kind() == reflect.Struct {
			if slices.Contains(splConf, "squash") {
				gen.templateStruct(value.Interface(), spaces)
			} else {
				gen.templateStructWithHeaders(fieldName, value.Interface(), spaces)
			}
		} else if value.Kind() == reflect.String {
			gen.sb.WriteString(fmt.Sprintf("%s%s = \"%v\"\n", spaceString, fieldName, value.Interface()))
		} else if value.Kind() == reflect.Map {
			gen.sb.WriteString(fmt.Sprintf("%s%s = {}\n", spaceString, fieldName))
		} else if value.Kind() == reflect.Slice {
			gen.sb.WriteString(fmt.Sprintf("%s%s = []\n", spaceString, fieldName))
		} else {
			gen.sb.WriteString(fmt.Sprintf("%s%s = %v\n", spaceString, fieldName, value.Interface()))
		}
	}
}

type FieldDescriptor struct {
	Name        string
	Default     string
	Description string
}

func (gen *Generator) describeStruct(s interface{}) string {
	t := reflect.TypeOf(s)
	v := reflect.ValueOf(s)

	if t.Kind() != reflect.Struct {
		return ""
	}

	descriptors := []FieldDescriptor{}
	tp := &TablePrinter{}

	for i := 0; i < t.NumField(); i++ {
		fd := FieldDescriptor{}
		field := t.Field(i)
		value := v.Field(i)

		splConf := strings.Split(field.Tag.Get("mapstructure"), ",")
		fd.Name = splConf[0]
		fd.Description = field.Tag.Get("desc")

		if !value.IsValid() || value.Kind() == reflect.Ptr {
			if field.Type.Elem().Kind() == reflect.Struct {
				fd.Default = "{}"
			} else if value.IsNil() {
				fd.Default = "null"
			} else if field.Type.Elem().Kind() == reflect.String {
				fd.Default = fmt.Sprintf("\"%v\"", value.Interface())
			} else {
				fd.Default = fmt.Sprintf("%v\n", value.Interface())
			}

			continue
		}

		if value.Kind() == reflect.Struct {
			fd.Default = "{}"
		} else if value.Kind() == reflect.String {
			fd.Default = fmt.Sprintf("\"%v\"", value.Interface())
		} else if value.Kind() == reflect.Map {
			fd.Default = "{}"
		} else if value.Kind() == reflect.Slice {
			fd.Default = "[]"
		} else {
			fd.Default = fmt.Sprintf("%v", value.Interface())
		}

		tp.CheckFieldDescriptor(fd)
		descriptors = append(descriptors, fd)
	}

	headerDesc := FieldDescriptor{
		Name:        "Name",
		Default:     "Default",
		Description: "Description",
	}

	tp.CheckFieldDescriptor(headerDesc)
	gen.sb.WriteString(tp.PrintFieldDescriptor(headerDesc))
	gen.sb.WriteString(tp.GenerateHeaderLine())
	for _, d := range descriptors {
		gen.sb.WriteString(tp.PrintFieldDescriptor(d))
	}

	return gen.sb.String()
}

type TablePrinter struct {
	name int
	def  int
	desc int
}

func (tp *TablePrinter) CheckFieldDescriptor(fd FieldDescriptor) {
	if len(fd.Name) > tp.name {
		tp.name = len(fd.Name)
	}
	if len(fd.Description) > tp.desc {
		tp.desc = len(fd.Description)
	}
	if len(fd.Default) > tp.def {
		tp.def = len(fd.Default)
	}
}

func (tp *TablePrinter) GenerateHeaderLine() string {
	var sb strings.Builder
	for i := 0; i < tp.name+1; i++ {
		sb.WriteString("-")
	}
	sb.WriteString("|-")
	for i := 0; i < tp.def+1; i++ {
		sb.WriteString("-")
	}
	sb.WriteString("|-")
	for i := 0; i < tp.desc+1; i++ {
		sb.WriteString("-")
	}

	sb.WriteString("\n")
	return sb.String()
}

func (tp *TablePrinter) PrintFieldDescriptor(fd FieldDescriptor) string {
	var sb strings.Builder
	sb.WriteString(fd.Name)
	for i := 0; i < tp.name-len(fd.Name)+1; i++ {
		sb.WriteString(" ")
	}
	sb.WriteString("| ")
	sb.WriteString(fd.Default)
	for i := 0; i < tp.def-len(fd.Default)+1; i++ {
		sb.WriteString(" ")
	}
	sb.WriteString("| ")
	sb.WriteString(fd.Description)
	for i := 0; i < tp.desc-len(fd.Description)+1; i++ {
		sb.WriteString(" ")
	}

	sb.WriteString("\n")
	return sb.String()
}
