package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/mackerelio/mackerel-client-go"
	"github.com/urfave/cli/v2"
)

var privateIPNets = map[string]*net.IPNet{
	"10.0.0.0/8":     &net.IPNet{IP: net.IP{0xa, 0x0, 0x0, 0x0}, Mask: net.IPMask{0xff, 0x0, 0x0, 0x0}},
	"172.16.0.0/12":  &net.IPNet{IP: net.IP{0xac, 0x10, 0x0, 0x0}, Mask: net.IPMask{0xff, 0xf0, 0x0, 0x0}},
	"192.168.0.0/16": &net.IPNet{IP: net.IP{0xc0, 0xa8, 0x0, 0x0}, Mask: net.IPMask{0xff, 0xff, 0x0, 0x0}},
}

func isPrivateIP(ip string) bool {
	for _, ipNet := range privateIPNets {
		if ipNet.Contains(net.ParseIP(ip)) {
			return true
		}
	}
	return false
}

type Inventory struct {
	mackerelClient *mackerel.Client
	addedHosts     map[string]bool
	Groups         map[string][]string
	Meta           map[string]map[string]interface{}
}

type HostVars struct {
	Hosts map[string]interface{}
}

func NewInventory(mackerelClient *mackerel.Client) *Inventory {
	return &Inventory{
		mackerelClient: mackerelClient,
		addedHosts:     map[string]bool{},
		Groups:         map[string][]string{},
		Meta:           map[string]map[string]interface{}{},
	}
}

func (i *Inventory) List() string {
	hosts, err := i.mackerelClient.FindHosts(&mackerel.FindHostsParam{})
	if err != nil {
		return `{"_meta":{"hostvars":{}}}`
	}
	for _, host := range hosts {
		i.addHost(host)
	}

	b, err := json.Marshal(i)
	if err != nil {
		return `{"_meta":{"hostvars":{}}}`
	}
	return string(b)
}

func (i *Inventory) Host(name string) string {
	hosts, err := i.mackerelClient.FindHosts(&mackerel.FindHostsParam{Name: name})
	if err != nil {
		return "{}"
	}
	for _, host := range hosts {
		i.addHost(host)
	}

	for k, vars := range i.Meta {
		if k == name {
			b, err := json.Marshal(vars)
			if err != nil {
				return "{}"
			}
			return string(b)
		}
	}

	return "{}"
}

func (i *Inventory) MarshalJSON() ([]byte, error) {
	data := make(map[string]interface{})
	data["_meta"] = struct {
		HostVars map[string]map[string]interface{} `json:"hostvars"`
	}{
		HostVars: i.Meta,
	}
	for k, v := range i.Groups {
		data[k] = v
	}

	return json.Marshal(data)
}

func (i *Inventory) addHost(host *mackerel.Host) {
	if i.addedHosts[host.Name] {
		return
	}

	for serviceName, roles := range host.Roles {
		// Grouping by service
		if serviceName != "" {
			i.Groups[serviceName] = append(i.Groups[serviceName], host.Name)
		}

		// Grouping by role
		for _, role := range roles {
			if role != "" {
				i.Groups[role] = append(i.Groups[role], host.Name)
			}
		}
	}
	// Grouping by type
	if host.Type != "" {
		i.Groups[host.Type] = append(i.Groups[host.Type], host.Name)
	}

	// Grouping by status
	if host.Status != "" {
		i.Groups[host.Status] = append(i.Groups[host.Status], host.Name)
	}

	for _, ifs := range host.Interfaces {
		if isPrivateIP(ifs.IPAddress) {
			i.Meta[host.Name] = map[string]interface{}{
				"ansible_host": ifs.IPAddress,
			}
		}
	}

	i.addedHosts[host.Name] = true
}

func main() {
	app := &cli.App{
		Name: "mackerel",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "list",
				Usage: "List hosts",
			},
			&cli.StringFlag{
				Name:  "host",
				Usage: "Get all the variables about a specific `HOST_NAME`",
			},
			&cli.StringFlag{
				Name:    "mackerel-api-key",
				Usage:   "`API_KEY` for mackrel.io",
				EnvVars: []string{"MACKEREL_API_KEY"},
			},
		},
		Action: func(c *cli.Context) error {
			apiKey := c.String("mackerel-api-key")
			if apiKey == "" {
				return cli.Exit("Error: mackerel-api-key is require", 1)
			}
			client := mackerel.NewClient(c.String("mackerel-api-key"))
			i := NewInventory(client)

			if c.Bool("list") {
				fmt.Fprintln(c.App.Writer, i.List())
				return nil
			}
			if host := c.String("host"); host != "" {
				fmt.Fprintln(c.App.Writer, i.Host(host))
				return nil
			}

			return nil
		},
	}
	if err := app.Run(os.Args); err != nil {
		cli.HandleExitCoder(err)
	}
}
