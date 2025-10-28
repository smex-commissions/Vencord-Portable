package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var PackageJson = `{
	"name": "discord",
	"main": "index.js"
}`

type asarEntry struct {
	Size   int32  `json:"size"`
	Offset string `json:"offset"`
}

// Ported from https://github.com/GeopJr/asar-cr/blob/cd7695b7c913bf921d9fb6600eaeb1400e3ba225/src/asar-cr/pack.cr#L61

func WriteAppAsar(outFile string, vencordAsarPath string, configPath string) error {
	header := make(map[string]map[string]asarEntry)
	files := make(map[string]asarEntry)
	header["files"] = files

	fileContents := ""

	patcherPathB, _ := json.Marshal(vencordAsarPath)
	indexBuilder := &strings.Builder{}
	indexBuilder.WriteString("\"use strict\";\n")

	if configPath != "" {
		configPathB, _ := json.Marshal(configPath)
		indexBuilder.WriteString("const fs = require(\"fs\");\n")
		indexBuilder.WriteString("const childProcess = require(\"child_process\");\n")
		indexBuilder.WriteString("const { shell } = require(\"electron\");\n")
		indexBuilder.WriteString("const originalOpenExternal = shell.openExternal.bind(shell);\n")
		indexBuilder.WriteString("let portableBrowserPath = \"\";\n")
		indexBuilder.WriteString("try {\n")
		indexBuilder.WriteString("        const rawConfig = fs.readFileSync(" + string(configPathB) + ", \"utf8\");\n")
		indexBuilder.WriteString("        const parsedConfig = JSON.parse(rawConfig);\n")
		indexBuilder.WriteString("        if (parsedConfig && typeof parsedConfig.browserPath === \"string\" && parsedConfig.browserPath.length > 0) {\n")
		indexBuilder.WriteString("                portableBrowserPath = parsedConfig.browserPath;\n")
		indexBuilder.WriteString("        }\n")
		indexBuilder.WriteString("} catch (error) {\n")
		indexBuilder.WriteString("        if (error && error.code !== \"ENOENT\") {\n")
		indexBuilder.WriteString("                console.error(\"Failed to load portable browser configuration:\", error);\n")
		indexBuilder.WriteString("        }\n")
		indexBuilder.WriteString("}\n")
		indexBuilder.WriteString("if (portableBrowserPath) {\n")
		indexBuilder.WriteString("        shell.openExternal = (target, options) => {\n")
		indexBuilder.WriteString("                return new Promise((resolve) => {\n")
		indexBuilder.WriteString("                        try {\n")
		indexBuilder.WriteString("                                const child = childProcess.spawn(portableBrowserPath, [String(target ?? \"\")], {\n")
		indexBuilder.WriteString("                                        detached: true,\n")
		indexBuilder.WriteString("                                        stdio: \"ignore\",\n")
		indexBuilder.WriteString("                                });\n")
		indexBuilder.WriteString("                                child.once(\"error\", (spawnError) => {\n")
		indexBuilder.WriteString("                                        console.error(\"Portable browser launch error:\", spawnError);\n")
		indexBuilder.WriteString("                                        resolve(originalOpenExternal(target, options));\n")
		indexBuilder.WriteString("                                });\n")
		indexBuilder.WriteString("                                child.unref();\n")
		indexBuilder.WriteString("                                resolve(true);\n")
		indexBuilder.WriteString("                        } catch (launchError) {\n")
		indexBuilder.WriteString("                                console.error(\"Failed to launch portable browser:\", launchError);\n")
		indexBuilder.WriteString("                                resolve(originalOpenExternal(target, options));\n")
		indexBuilder.WriteString("                        }\n")
		indexBuilder.WriteString("                });\n")
		indexBuilder.WriteString("        };\n")
		indexBuilder.WriteString("}\n")
	}
	indexBuilder.WriteString("require(" + string(patcherPathB) + ")\n")
	indexJsContents := indexBuilder.String()
	indexJsBytes := len([]byte(indexJsContents))
	fileContents += indexJsContents
	files["index.js"] = asarEntry{
		Size:   int32(indexJsBytes),
		Offset: "0",
	}

	fileContents += PackageJson
	files["package.json"] = asarEntry{
		Size:   int32(len([]byte(PackageJson))),
		Offset: strconv.Itoa(indexJsBytes),
	}

	headerBytes, _ := json.Marshal(header)
	headerString := string(headerBytes)
	headerStringSize := uint32(len(headerString))
	dataSize := uint32(4)
	alignedSize := (headerStringSize + dataSize - 1) & ^(dataSize - 1)
	headerSize := alignedSize + 8
	headerObjectSize := alignedSize + dataSize
	diff := alignedSize - headerStringSize
	if diff > 0 {
		headerString += strings.Repeat("0", int(diff))
	}

	f, err := os.Create(outFile)
	if err != nil {
		return fmt.Errorf("Failed to create %s: %w", outFile, err)
	}
	defer f.Close()

	for _, n := range []uint32{dataSize, headerSize, headerObjectSize, headerStringSize} {
		if err = binary.Write(f, binary.LittleEndian, int32(n)); err != nil {
			return fmt.Errorf("Failed to write asar bytes: %w", err)
		}
	}

	for _, s := range []string{headerString, fileContents} {
		if _, err = f.WriteString(s); err != nil {
			return fmt.Errorf("Failed to write asar data: %w", err)
		}
	}

	return nil
}
