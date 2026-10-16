# DHCP Override Contract

No new CRD fields or conditions. Existing v1alpha6 fields retain their types, optionality, and update restrictions.

| Field | Enabled behavior |
|-------|------------------|
| `spec.network.interfaces[].nameservers` | Unchanged. Rendered as before; does not suppress DHCP DNS. |
| `spec.network.interfaces[].gateway4` / `gateway6` | `None` also suppresses DHCP routes for that family with CloudInit. Concrete IPs remain incompatible with same-family DHCP. |
| `spec.network.interfaces[].routes` | Explicit CloudInit routes remain installed; do not independently suppress DHCP routes. |

LinuxPrep and Sysprep behavior is unchanged.

When both effective DHCP modes are enabled, CloudInit requires matching route suppression.

```yaml
spec:
  network:
    interfaces:
    - name: eth0
      dhcp4: true
      dhcp6: true
      gateway4: None
      gateway6: None
      routes:
      - to: 198.51.100.0/24
        via: 192.0.2.1
```

With CloudInit, both DHCP override maps contain `use-routes: false`; the explicit static route remains. There are no conversion-webhook changes because the serialized API shape is unchanged.
