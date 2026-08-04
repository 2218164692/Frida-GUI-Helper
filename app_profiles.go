package main

import (
	"errors"

	"frida-gui-helper/internal/profiles"
)

func (a *App) ListProfiles() ([]profiles.Profile, error) {
	if a.profiles == nil {
		return nil, errors.New("运行配置存储未初始化")
	}
	return a.profiles.List()
}

func (a *App) SaveProfile(req profiles.SaveRequest) (profiles.Profile, error) {
	if a.profiles == nil {
		return profiles.Profile{}, errors.New("运行配置存储未初始化")
	}
	profile, err := a.profiles.Save(req)
	if err != nil {
		a.addLog("error", "profiles", "保存运行配置失败: "+err.Error())
		return profiles.Profile{}, err
	}
	a.addLog("info", "profiles", "已保存运行配置: "+profile.Name)
	return profile, nil
}

func (a *App) DeleteProfile(id string) error {
	if a.profiles == nil {
		return errors.New("运行配置存储未初始化")
	}
	if err := a.profiles.Delete(id); err != nil {
		return err
	}
	a.addLog("info", "profiles", "已删除运行配置")
	return nil
}

func (a *App) GetLastProfile() (*profiles.Profile, error) {
	if a.profiles == nil {
		return nil, errors.New("运行配置存储未初始化")
	}
	return a.profiles.Last()
}

func (a *App) MarkProfileRun(id string) (profiles.Profile, error) {
	if a.profiles == nil {
		return profiles.Profile{}, errors.New("运行配置存储未初始化")
	}
	return a.profiles.MarkRun(id)
}
