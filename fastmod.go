// Copyright 2018 visualfc <visualfc@gmail.com>. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// internal modfile/module/semver copy from Go1.11 source

package fastmod

import (
	"go/build"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/visualfc/fastmod/internal/modfile"
)

var (
	PkgModPath string
)

func UpdatePkgMod(ctx *build.Context) {
	if list := filepath.SplitList(ctx.GOPATH); len(list) > 0 && list[0] != "" {
		PkgModPath = filepath.Join(list[0], "pkg/mod")
	}
}

func fixVersion(path, vers string) (string, error) {
	return vers, nil
}

func LookupModFile(dir string) (string, error) {
	command := exec.Command("go", "env", "GOMOD")
	command.Dir = dir
	data, err := command.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

type ModuleList struct {
	mods map[string]*Module
}

func NewModuleList(ctx *build.Context) *ModuleList {
	UpdatePkgMod(ctx)
	return &ModuleList{make(map[string]*Module)}
}

type Version struct {
	Path    string
	Version string
}

type Mod struct {
	Require *Version
	Replace *Version
}

// check mod path
func (m *Mod) Path() string {
	v := m.Require
	if m.Replace != nil {
		v = m.Replace
	}
	if v.Version != "" {
		return v.Path + "@" + v.Version
	}
	return v.Path
}

type Module struct {
	f     *modfile.File
	ftime int64
	path  string
	fmod  string
	fdir  string
	mods  []*Mod
}

func (m *Module) init() {
	rused := make(map[int]bool)
	for _, r := range m.f.Require {
		mod := &Mod{Require: &Version{r.Mod.Path, r.Mod.Version}}
		for i, v := range m.f.Replace {
			if r.Mod.Path == v.Old.Path && (v.Old.Version == "" || v.Old.Version == r.Mod.Version) {
				mod.Replace = &Version{v.New.Path, v.New.Version}
				rused[i] = true
				break
			}
		}
		m.mods = append(m.mods, mod)
	}
	for i, v := range m.f.Replace {
		if rused[i] {
			continue
		}
		mod := &Mod{Require: &Version{v.Old.Path, v.Old.Version}, Replace: &Version{v.New.Path, v.New.Version}}
		m.mods = append(m.mods, mod)
	}
}

func (m *Module) Path() string {
	return m.f.Module.Mod.Path
}

func (m *Module) ModFile() string {
	return m.fmod
}

func (m *Module) ModDir() string {
	return m.fdir
}

type PkgType int

const (
	PkgTypeNil      PkgType = iota
	PkgTypeGoroot           // goroot pkg
	PkgTypeGopath           // gopath pkg
	PkgTypeMod              // mod pkg
	PkgTypeLocal            // mod pkg sub local
	PkgTypeLocalMod         // mod pkg sub local mod
	PkgTypeDepMod           // mod pkg dep gopath/pkg/mod
)

func (m *Module) Lookup(pkg string) (path string, dir string, typ PkgType) {
	if strings.HasPrefix(pkg, m.path+"/") {
		return pkg, filepath.Join(m.fdir, pkg[len(m.path+"/"):]), PkgTypeLocal
	}

	for _, r := range m.mods {
		if r.Require.Path == pkg {
			path = r.Path()
			break
		} else if strings.HasPrefix(pkg, r.Require.Path+"/") {
			path = r.Path() + pkg[len(r.Require.Path):]
			break
		}
	}
	if path == "" {
		return "", "", PkgTypeNil
	}
	if strings.HasPrefix(path, "./") {
		return pkg, filepath.Join(m.fdir, path), PkgTypeLocalMod
	}
	return pkg, filepath.Join(PkgModPath, path), PkgTypeDepMod
}

func (mc *ModuleList) LoadModule(dir string) (*Module, error) {
	fmod, err := LookupModFile(dir)
	if fmod == "" {
		return nil, err
	}
	info, _ := os.Stat(fmod)
	if m, ok := mc.mods[fmod]; ok {
		if m.ftime == info.ModTime().UnixNano() {
			return m, nil
		}
	}
	data, err := ioutil.ReadFile(fmod)
	if err != nil {
		return nil, err
	}
	f, err := modfile.Parse(fmod, data, fixVersion)
	if err != nil {
		return nil, err
	}
	m := &Module{f, info.ModTime().UnixNano(), f.Module.Mod.Path, fmod, filepath.Dir(fmod), nil}
	m.init()
	mc.mods[fmod] = m
	return m, nil
}
