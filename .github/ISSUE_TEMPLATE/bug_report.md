---

name: Bug report
about: Tell us about a problem you are experiencing

---

<!--
Thank you for the bug report! Before submitting, please delete all of
the HTML comments. Thank you for taking the time to help us improve the
project!
-->

**What steps did you take and what happened?**

<!-- A clear and concise description of what the bug is. -->


**What did you expect to happen?**

<!-- Please describe what you expected the outcome to be. -->


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

