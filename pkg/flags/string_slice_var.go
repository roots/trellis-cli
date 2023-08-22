package flags

type StringSliceVar []string

func (f *StringSliceVar) String() string {
	return ""
}

func (f *StringSliceVar) Set(value string) error {
	*f = append(*f, value)
	return nil
}
