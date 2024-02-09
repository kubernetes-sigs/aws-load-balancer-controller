<p>Packages:</p>
<ul>
<li>
<a href="#elbv2.k8s.aws%2fv1beta1">elbv2.k8s.aws/v1beta1</a>
</li>
</ul>
<h2 id="elbv2.k8s.aws/v1beta1">elbv2.k8s.aws/v1beta1</h2>
<p>
<p>Package v1beta1 contains API Schema definitions for the elbv2 v1beta1 API group</p>
</p>
Resource Types:
<ul><li>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBinding">TargetGroupBinding</a>
</li></ul>
<h3 id="elbv2.k8s.aws/v1beta1.TargetGroupBinding">TargetGroupBinding
</h3>
<p>
<p>TargetGroupBinding is the Schema for the TargetGroupBinding API</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>apiVersion</code></br>
string</td>
<td>
<code>
elbv2.k8s.aws/v1beta1
</code>
</td>
</tr>
<tr>
<td>
<code>kind</code></br>
string
</td>
<td><code>TargetGroupBinding</code></td>
</tr>
<tr>
<td>
<code>metadata</code></br>
<em>
<a href="https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.16/#objectmeta-v1-meta">
Kubernetes meta/v1.ObjectMeta
</a>
</em>
</td>
<td>
Refer to the Kubernetes API documentation for the fields of the
<code>metadata</code> field.
</td>
</tr>
<tr>
<td>
<code>spec</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingSpec">
TargetGroupBindingSpec
</a>
</em>
</td>
<td>
<br/>
<br/>
<table>
<tr>
<td>
<code>targetGroupARN</code></br>
<em>
string
</em>
</td>
<td>
<p>targetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.</p>
</td>
</tr>
<tr>
<td>
<code>targetType</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetType">
TargetType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred.</p>
</td>
</tr>
<tr>
<td>
<code>serviceRef</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.ServiceReference">
ServiceReference
</a>
</em>
</td>
<td>
<p>serviceRef is a reference to a Kubernetes Service and ServicePort.</p>
</td>
</tr>
<tr>
<td>
<code>networking</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingNetworking">
TargetGroupBindingNetworking
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>networking defines the networking rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.</p>
</td>
</tr>
</table>
</td>
</tr>
<tr>
<td>
<code>status</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingStatus">
TargetGroupBindingStatus
</a>
</em>
</td>
<td>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.IPBlock">IPBlock
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingPeer">NetworkingPeer</a>)
</p>
<p>
<p>IPBlock defines source/destination IPBlock in networking rules.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>cidr</code></br>
<em>
string
</em>
</td>
<td>
<p>CIDR is the network CIDR.
Both IPV4 or IPV6 CIDR are accepted.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.NetworkingIngressRule">NetworkingIngressRule
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingNetworking">TargetGroupBindingNetworking</a>)
</p>
<p>
<p>NetworkingIngressRule defines a particular set of traffic that is allowed to access TargetGroup&rsquo;s targets.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>from</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingPeer">
[]NetworkingPeer
</a>
</em>
</td>
<td>
<p>List of peers which should be able to access the targets in TargetGroup.
At least one NetworkingPeer should be specified.</p>
</td>
</tr>
<tr>
<td>
<code>ports</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingPort">
[]NetworkingPort
</a>
</em>
</td>
<td>
<p>List of ports which should be made accessible on the targets in TargetGroup.
If ports is empty or unspecified, it defaults to all ports with TCP.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.NetworkingPeer">NetworkingPeer
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingIngressRule">NetworkingIngressRule</a>)
</p>
<p>
<p>NetworkingPeer defines the source/destination peer for networking rules.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ipBlock</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.IPBlock">
IPBlock
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>IPBlock defines an IPBlock peer.
If specified, none of the other fields can be set.</p>
</td>
</tr>
<tr>
<td>
<code>securityGroup</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.SecurityGroup">
SecurityGroup
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>SecurityGroup defines a SecurityGroup peer.
If specified, none of the other fields can be set.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.NetworkingPort">NetworkingPort
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingIngressRule">NetworkingIngressRule</a>)
</p>
<p>
<p>NetworkingPort defines the port and protocol for networking rules.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>protocol</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingProtocol">
NetworkingProtocol
</a>
</em>
</td>
<td>
<p>The protocol which traffic must match.
If protocol is unspecified, it defaults to TCP.</p>
</td>
</tr>
<tr>
<td>
<code>port</code></br>
<em>
k8s.io/apimachinery/pkg/util/intstr.IntOrString
</em>
</td>
<td>
<em>(Optional)</em>
<p>The port which traffic must match.
When NodePort endpoints(instance TargetType) is used, this must be a numerical port.
When Port endpoints(ip TargetType) is used, this can be either numerical or named port on pods.
if port is unspecified, it defaults to all ports.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.NetworkingProtocol">NetworkingProtocol
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingPort">NetworkingPort</a>)
</p>
<p>
<p>NetworkingProtocol defines the protocol for networking rules.</p>
</p>
<h3 id="elbv2.k8s.aws/v1beta1.SecurityGroup">SecurityGroup
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingPeer">NetworkingPeer</a>)
</p>
<p>
<p>SecurityGroup defines reference to an AWS EC2 SecurityGroup.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>groupID</code></br>
<em>
string
</em>
</td>
<td>
<p>GroupID is the EC2 SecurityGroupID.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.ServiceReference">ServiceReference
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingSpec">TargetGroupBindingSpec</a>)
</p>
<p>
<p>ServiceReference defines reference to a Kubernetes Service and its ServicePort.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>name</code></br>
<em>
string
</em>
</td>
<td>
<p>Name is the name of the Service.</p>
</td>
</tr>
<tr>
<td>
<code>port</code></br>
<em>
k8s.io/apimachinery/pkg/util/intstr.IntOrString
</em>
</td>
<td>
<p>Port is the port of the ServicePort.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.TargetGroupBindingNetworking">TargetGroupBindingNetworking
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingSpec">TargetGroupBindingSpec</a>)
</p>
<p>
<p>TargetGroupBindingNetworking defines the networking rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>ingress</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.NetworkingIngressRule">
[]NetworkingIngressRule
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>List of ingress rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.TargetGroupBindingSpec">TargetGroupBindingSpec
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBinding">TargetGroupBinding</a>)
</p>
<p>
<p>TargetGroupBindingSpec defines the desired state of TargetGroupBinding</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>targetGroupARN</code></br>
<em>
string
</em>
</td>
<td>
<p>targetGroupARN is the Amazon Resource Name (ARN) for the TargetGroup.</p>
</td>
</tr>
<tr>
<td>
<code>targetType</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetType">
TargetType
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>targetType is the TargetType of TargetGroup. If unspecified, it will be automatically inferred.</p>
</td>
</tr>
<tr>
<td>
<code>serviceRef</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.ServiceReference">
ServiceReference
</a>
</em>
</td>
<td>
<p>serviceRef is a reference to a Kubernetes Service and ServicePort.</p>
</td>
</tr>
<tr>
<td>
<code>networking</code></br>
<em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingNetworking">
TargetGroupBindingNetworking
</a>
</em>
</td>
<td>
<em>(Optional)</em>
<p>networking defines the networking rules to allow ELBV2 LoadBalancer to access targets in TargetGroup.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.TargetGroupBindingStatus">TargetGroupBindingStatus
</h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBinding">TargetGroupBinding</a>)
</p>
<p>
<p>TargetGroupBindingStatus defines the observed state of TargetGroupBinding</p>
</p>
<table>
<thead>
<tr>
<th>Field</th>
<th>Description</th>
</tr>
</thead>
<tbody>
<tr>
<td>
<code>observedGeneration</code></br>
<em>
int64
</em>
</td>
<td>
<em>(Optional)</em>
<p>The generation observed by the TargetGroupBinding controller.</p>
</td>
</tr>
</tbody>
</table>
<h3 id="elbv2.k8s.aws/v1beta1.TargetType">TargetType
(<code>string</code> alias)</p></h3>
<p>
(<em>Appears on:</em>
<a href="#elbv2.k8s.aws/v1beta1.TargetGroupBindingSpec">TargetGroupBindingSpec</a>)
</p>
<p>
<p>TargetType is the targetType of your ELBV2 TargetGroup.</p>
<ul>
<li>with <code>instance</code> TargetType, nodes with nodePort for your service will be registered as targets</li>
<li>with <code>ip</code> TargetType, Pods with containerPort for your service will be registered as targets</li>
</ul>
</p>
<hr/>
<p><em>
Generated with <code>gen-crd-api-reference-docs</code>
on git commit <code>21418f44</code>.
</em></p>
