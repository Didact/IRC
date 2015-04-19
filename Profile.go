package irc

type Profile struct {
	nname string
	uname string
	rname string
	autos []string
}

func NewProfile(nname, uname, rname string, autos []string) *Profile {
	this := new(Profile)
	this.nname = nname
	this.uname = uname
	this.rname = rname
	this.autos = autos
	return this
}
