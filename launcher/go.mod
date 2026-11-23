module github.com/renereose/varu/launcher

go 1.22

require (
	github.com/renereose/varu/router v0.0.0-00010101000000-000000000000
	github.com/renereose/varu/zim v0.0.0-00010101000000-000000000000
)

require github.com/klauspost/compress v1.18.0 // indirect

replace (
	github.com/renereose/varu/router => ../router
	github.com/renereose/varu/zim => ../zim
)
