package main

type NullWriter struct {
}

func (NullWriter) Write(p []byte) (n int, err error) {
	return 0, nil
}

func NewNullWriter() NullWriter {
	return NullWriter{}
}
