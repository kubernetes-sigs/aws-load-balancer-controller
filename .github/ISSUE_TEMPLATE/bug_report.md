---
name: Bug report
about: Create a report to help us improve
title: ''
labels: ''
assignees: ''

---

<!--- ❤️ Thanks for taking the time to report this issue. Before you open an issue, please check if a similar issue [already exists](https://github.com/kubernetes-sigs/aws-load-balancer-controller/issues) or has been closed before. If so, add additional helpful details to the existing issue to show that it's affecting multiple people. If not, to help us investigate, please provide the following information. ❤️-->

<!-- 🚨 IMPORTANT!!!
Please complete at least the following sections
- Bug Description
- Steps to Reproduce
- Expected Behavior
- Environment 
Issue missing details will be closed, as they are crucial for understand the problem. 🚨 -->

**Bug Description**
<!--- A concise description of what the bug is -->

**Steps to Reproduce**
<!--- Provide a step-by-step guide to reproduce the bug. If relevant, provide the controller logs with any error messages you are seeing and relevant Kubernetes Manifests.-->
- Step-by-step guide to reproduce the bug:
- Manifests applied while reproducing the issue:
- Controller logs/error messages while reproducing the issue:

**Expected Behavior**
<!--- Describe what you expected to happen instead of the observed behavior -->

**Actual Behavior**
<!--- Describe what actually happens, including details on 
- whether the bug causes the controller to stop working entirely or impact some functionality
- how often this bug occur ? [e.g., Always / Often / Occasionally / Rarely]
-->

**Regression**
Was the functionality working correctly in a previous version ? [Yes / No]
If yes, specify the last version where it worked as expected

**Current Workarounds**
<!--- If any workarounds exist, describe them here. If none exist, state "No workarounds available" -->

**Environment**
- AWS Load Balancer controller version:
- Kubernetes version:
- Using EKS (yes/no), if so version?:
- Using Service or Ingress:
- AWS region:
- How was the aws-load-balancer-controller installed:
  - If helm was used then please show output of `helm ls -A | grep -i aws-load-balancer-controller`
  - If helm was used then please show output of `helm -n <controllernamespace> get values <helmreleasename>`
  - If helm was not used, then copy/paste the exact command used to install the controller, including flags and options.
- Current state of the Controller configuration:
  - `kubectl -n <controllernamespace> describe deployment aws-load-balancer-controller`
- Current state of the Ingress/Service configuration:
  - `kubectl describe ingressclasses`
  - `kubectl -n <appnamespace> describe ingress <ingressname>`
  - `kubectl -n <appnamespace> describe svc <servicename>`

**Possible Solution (Optional)**
<!--- If you have insights into the cause or potential fix, please share them. -->

**Contribution Intention (Optional)**
<!---If the solution is accepted, would you be willing to submit a PR?
- if yes, please follow https://github.com/kubernetes-sigs/aws-load-balancer-controller/blob/main/CONTRIBUTING.md to start your contribution.
- If you're not able to contribute a fix, the issue will be open for public contribution. High-impact issues will receive priority attention. We encourage community participation to help resolve issues faster. 
-->
- [ ] Yes, I'm willing to submit a PR to fix this issue
- [ ] No, I cannot work on a PR at this time

**Additional Context**
<!---Add any other context about the problem here.-->
