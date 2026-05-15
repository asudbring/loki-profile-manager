package storemigrate

import "io/fs"

const darwinDatalessFlag uint32 = 0x40000000

func datalessFlagSet(flags uint32) bool {
	return flags&darwinDatalessFlag != 0
}

func datalessBlocker(entry Entry) string {
	return "source file is cloud-only/dataless and must be downloaded before migration: " + entry.RelativePath
}

func fileInfoDataless(info fs.FileInfo) bool {
	return fileInfoDatalessForPlatform(info)
}
