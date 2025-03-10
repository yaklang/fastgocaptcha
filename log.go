package fastgocaptcha

func (f *FastGoCaptcha) SetInfof(infof func(format string, v ...any)) {
	f.infof = infof
}

func (f *FastGoCaptcha) SetWarningf(warningf func(format string, v ...any)) {
	f.warningf = warningf
}

func (f *FastGoCaptcha) SetErrorf(errorf func(format string, v ...any)) {
	f.errorf = errorf
}

func (f *FastGoCaptcha) logInfof(format string, v ...any) {
	if f.infof != nil {
		f.infof(format, v...)
	}
}

func (f *FastGoCaptcha) logWarningf(format string, v ...any) {
	if f.warningf != nil {
		f.warningf(format, v...)
	}
}

func (f *FastGoCaptcha) logErrorf(format string, v ...any) {
	if f.errorf != nil {
		f.errorf(format, v...)
	}
}
