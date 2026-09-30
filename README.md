# NUT Allergy

A NUT replacement for a LAN of SNMPv3 UPSs. One server polls the supplies. Linux agents on the machines shut them down when the supplies they depend on have been on battery long enough.

The server is a single linux/amd64 binary. The dashboard and the agent are already inside it.

## Install the server

Download `nut-allergy-amd64` from the [releases](https://github.com/jdbnet/nut-allergy/releases) page onto a machine that can reach the UPS network, then run it as root:

```sh
chmod +x nut-allergy-amd64
sudo ./nut-allergy-amd64
```

That copies the binary to `/usr/local/bin/nut-allergy-server`, writes a systemd unit, and starts it. Run the same command again to upgrade: the service is restarted, and enrolled agents replace themselves with the matching agent.

Open the first-run wizard at `http://<this-machine>:8080`.

## First-run wizard

The wizard is plain HTTP and only listens until setup is finished.

1. Choose an admin password (at least 8 characters).
2. Enter the DNS name browsers and agents will use, such as `ups.example.com`. It must contain a dot and resolve to this machine on the LAN.
3. Install a certificate. Let's Encrypt uses DNS-01 only, so the server never has to be reachable from the internet. Pick the DNS host (Cloudflare, DigitalOcean, Hetzner, Gandi, DNSimple, or Linode), paste an API token, and give a contact email. Or upload a certificate and key you already have.
4. Set how many seconds every bound UPS must stay on battery before agents shut down. 1800 (30 minutes) is the suggested value. Each agent can override it later.
5. Add the first UPS, or skip and add them from the dashboard.

After the certificate is installed the browser moves to `https://<your-hostname>/`. Keep using that address. The dashboard is the admin UI, not a site to publish.

## Add the UPSs

Use **Add UPS**. Each supply needs a name, an SNMP address, and SNMPv3 credentials: `authNoPriv` or `authPriv`, the username, the auth protocol (MD5 or a SHA variant), and the passwords. The server polls them and shows state, battery, load, and input voltage on the fleet page.

## Install an agent

On the fleet page, **Copy install line** and run that command as root on each linux/amd64 machine that should shut down. It installs `/usr/local/bin/nut-allergy-agent`, enrolls with a one-time token, and starts `nut-allergy-agent.service`.

Open the agent in the dashboard and tick the UPSs that feed that machine. An agent shuts down only when every UPS assigned to it is on battery and the timeout has elapsed. If one of those supplies is still on utility, it stays up. Low battery shuts down immediately, as long as every assigned UPS is already on battery. The shutdown command is `/sbin/shutdown -h +0`.

If the link to the server drops after a shutdown time was already set, the agent still powers off at that time. If the link drops while utility power was good, it does not.

## Where things live

| Path | What it is |
| --- | --- |
| `/usr/local/bin/nut-allergy-server` | Server binary |
| `/etc/systemd/system/nut-allergy-server.service` | Server service |
| `/var/lib/nut-allergy` | Database, encryption key, and certificates |
| `/usr/local/bin/nut-allergy-agent` | Agent binary, on each protected machine |
| `/etc/nut-allergy/agent.json` | Agent config, pushed from the server |
| `/etc/systemd/system/nut-allergy-agent.service` | Agent service |

Logs: `journalctl -u nut-allergy-server` and `journalctl -u nut-allergy-agent`.