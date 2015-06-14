package agario

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func GetCurrentLocation() (currentLocation string, recommendedServer string, err error) {
	resp, err := http.Get("http://gc.agar.io/")
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	locBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	loc := string(locBytes)

	locParts := strings.Split(loc, " ")
	newLocParts := make([]string, 0, len(locParts))
	for _, l := range locParts {
		if l == "?" {
			continue
		}

		newLocParts = append(newLocParts, l)
	}

	loc = strings.Join(newLocParts, " ")

	return loc, gcMap[loc], nil
}

func GetInfo() (*Info, error) {
	resp, err := http.Get("http://m.agar.io/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	internalInfo := new(internalInfo)

	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(internalInfo); err != nil {
		return nil, err
	}

	return cleanupInfo(internalInfo), nil
}

type internalInfo struct {
	Totals struct {
		Players int `json:"numPlayers"`
		Realms  int `json:"numRealms"`
		Servers int `json:"numServers"`
	}

	Regions map[string]struct {
		Players int `json:"numPlayers"`
		Realms  int `json:"numRealms"`
		Servers int `json:"numServers"`
	}
}

func cleanupInfo(internal *internalInfo) *Info {
	info := &Info{
		Regions: make([]*Region, 0, len(internal.Regions)),
	}

	for regionID, region := range internal.Regions {
		if regionID == "Unknown" {
			continue
		}

		nameSplit := strings.SplitN(regionID, ":", 2)

		gameMode := "ffa"
		if len(nameSplit) > 1 {
			gameMode = nameSplit[1]
		}

		region := &Region{
			Region:   nameSplit[0],
			GameMode: gameMode,

			Players: region.Players,
			Realms:  region.Realms,
			Servers: region.Servers,
		}

		info.Regions = append(info.Regions, region)
	}

	return info
}

type Info struct {
	Regions []*Region
}

type Region struct {
	Region   string
	GameMode string

	Players int
	Realms  int
	Servers int
}

func (r *Region) getServer() (net.Addr, error) {
	values := make(url.Values)
	gamemode := ""
	if r.GameMode != "ffa" {
		gamemode = ":" + r.GameMode
	}
	values.Set(r.Region+gamemode, "")

	resp, err := http.PostForm("http://m.agar.io/", values)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	addrBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	addrStr := string(addrBytes)
	if addrStr == "45.79.222.79:443" {
		// The default client ignores this address for some reason
		return r.getServer()
	}

	addr, err := net.ResolveTCPAddr("tcp", addrStr)
	if err != nil {
		return nil, err
	}

	return addr, nil
}

func (r *Region) Connect() (*Connection, error) {
	addr, err := r.getServer()
	if err != nil {
		return nil, err
	}

	c := new(Connection)
	err = c.connect(addr)
	if err != nil {
		return nil, err
	}

	return c, nil
}
