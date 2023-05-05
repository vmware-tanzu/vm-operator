---

name: Feature enhancement request
about: Suggest an idea for this project

---

<!--
Thank you for the feature request! Before submitting, please delete all of
the HTML comments. Thank you again, and we cannot wait to see what you are
going to suggest!
-->

**Please describe the solution you would like.**

<!-- A clear and concise description of what you want to happen. -->


**Is there anything else you would like to add?**

<!-- Miscellaneous information that will assist in solving the issue. -->


**Please tell us about your environment.**

<table>
<tr>
<td></td>
<th>Value</th>
<th>How to Obtain</th>
</tr>
<tr>
<th>Supervisor version</th>
<td><code><!-- Placeholder for the version --></code></td>
<td><code>rpm -qa VMware-wcp</code> on the vCenter appliance</td>
</tr>
<tr>
<th>Supervisor node image version</th>
<td><code><!-- Placeholder for the version --></code></td>
<td><code>rpm -qa VMware-wcpovf</code> on the vCenter appliance</td>
</tr>
<tr>
<th>Kubernetes version</th>
<td><code><!-- Placeholder for the version --></code></td>
<td><code>kubectl version</code></td>
</tr>
<tr>
<th>VM Operator version</th>
<td><code><!-- Placeholder for the version --></code></td>
<td>

```shell
kubectl -n vmware-system-vmop get pods \
  -ojsonpath='{range .items[*].spec.containers[*]}{.image}{"\n"}{end}' | \
  sort -u | \
  grep vmop | \
  awk -F'/' '{print $3}' | \
  awk -F: '{print $2}'
```

</td>
</tr>
</table>
