package a

import "time"

func bad() {
	_ = time.Now() // want "time.Now\\(\\) should be followed by .UTC\\(\\) for timezone consistency"
}

func good() {
	_ = time.Now().UTC()
}

func alsoBad() {
	t := time.Now() // want "time.Now\\(\\) should be followed by .UTC\\(\\) for timezone consistency"
	_ = t
}

func alsoGood() {
	t := time.Now().UTC()
	_ = t
}

func chainingGood() {
	_ = time.Now().UTC().Format(time.RFC3339)
}

func nolintGeneral() {
	//nolint
	_ = time.Now()
}

func nolintSpecific() {
	_ = time.Now() //nolint:timeutc
}

func nolintOtherLinter() {
	_ = time.Now() //nolint:otherlinter // want "time.Now\\(\\) should be followed by .UTC\\(\\) for timezone consistency"
}
