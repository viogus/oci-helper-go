package oci

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"
)

// TaskShapeForArchitecture returns the default OCI shape for a task
// architecture name. Unknown names are returned unchanged so callers can pass
// a concrete shape.
func TaskShapeForArchitecture(arch string) string {
	switch strings.ToUpper(strings.TrimSpace(arch)) {
	case "AMD":
		return "VM.Standard.E2.1.Micro"
	case "ARM":
		return "VM.Standard.A1.Flex"
	case "ARM_A2":
		return "VM.Standard.A2.Flex"
	case "AMD_E5":
		return "VM.Standard.E5.Flex"
	default:
		return arch
	}
}

// ListAllShapes lists all shapes available in a compartment without filtering
// by image. The OCI API accepts an empty imageId.
func (c *Client) ListAllShapes(ctx context.Context, compartmentID string) ([]core.Shape, error) {
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
	var all []core.Shape
	var page *string
	for {
		req := core.ListShapesRequest{
			CompartmentId: common.String(compartmentID),
			Limit:         common.Int(1000),
			Page:          page,
		}
		resp, err := c.compute.ListShapes(ctx, req)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return all, nil
}

// FindImageForOS returns the most recent image for an operating system name
// ("Ubuntu", "Oracle Linux", etc.).
func (c *Client) FindImageForOS(ctx context.Context, compartmentID, osName, shape string) (*core.Image, error) {
	osFilter := strings.TrimSpace(osName)
	if osFilter == "" {
		osFilter = "Ubuntu"
	}
	images, err := c.ListImages(ctx, compartmentID, osFilter)
	if err != nil {
		return nil, err
	}
	if len(images) == 0 {
		return nil, fmt.Errorf("no image found for OS %q", osFilter)
	}
	return &images[0], nil
}

// EnsurePublicSubnet finds an existing public subnet or provisions a minimal
// VCN + Internet Gateway + regional subnet so recurring create tasks work on
// tenants with no usable networking.
func (c *Client) EnsurePublicSubnet(ctx context.Context, compartmentID string) (*core.Subnet, error) {
	vcns, err := c.ListVCNs(ctx, compartmentID)
	if err != nil {
		return nil, fmt.Errorf("list vcns: %w", err)
	}
	for _, vcn := range vcns {
		if vcn.Id == nil || string(vcn.LifecycleState) != "AVAILABLE" {
			continue
		}
		if err := c.ensureInternetGateway(ctx, *vcn.Id); err != nil {
			log.Printf("[EnsurePublicSubnet] vcn %s internet gateway: %v", *vcn.Id, err)
			continue
		}
		subnets, err := c.ListSubnets(ctx, compartmentID, *vcn.Id)
		if err != nil {
			continue
		}
		for _, s := range subnets {
			if s.Id == nil || (s.ProhibitInternetIngress != nil && *s.ProhibitInternetIngress) {
				continue
			}
			return &s, nil
		}
	}
	return c.createDefaultVCNWithSubnet(ctx, compartmentID)
}

func (c *Client) ensureInternetGateway(ctx context.Context, vcnID string) error {
	igws, err := c.vcn.ListInternetGateways(ctx, core.ListInternetGatewaysRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
	})
	if err != nil {
		return err
	}
	var igwID string
	if len(igws.Items) > 0 && igws.Items[0].Id != nil {
		igwID = *igws.Items[0].Id
	} else {
		resp, err := c.vcn.CreateInternetGateway(ctx, core.CreateInternetGatewayRequest{
			CreateInternetGatewayDetails: core.CreateInternetGatewayDetails{
				CompartmentId: common.String(c.tenant.TenancyOCID),
				VcnId:         common.String(vcnID),
				IsEnabled:     common.Bool(true),
				DisplayName:   common.String("oci-helper-igw"),
			},
		})
		if err != nil {
			return err
		}
		if resp.InternetGateway.Id == nil {
			return fmt.Errorf("internet gateway created without id")
		}
		igwID = *resp.InternetGateway.Id
	}
	// Add 0.0.0.0/0 to the default route table so the subnet actually has
	// internet access.
	vcnResp, err := c.vcn.GetVcn(ctx, core.GetVcnRequest{VcnId: common.String(vcnID)})
	if err != nil {
		return err
	}
	rtID := vcnResp.Vcn.DefaultRouteTableId
	if rtID == nil {
		return nil
	}
	rt, err := c.vcn.GetRouteTable(ctx, core.GetRouteTableRequest{RtId: rtID})
	if err != nil {
		return err
	}
	for _, r := range rt.RouteRules {
		if r.Destination != nil && *r.Destination == "0.0.0.0/0" &&
			r.NetworkEntityId != nil && *r.NetworkEntityId == igwID {
			return nil
		}
	}
	rules := append(rt.RouteRules, core.RouteRule{
		Destination:     common.String("0.0.0.0/0"),
		DestinationType: core.RouteRuleDestinationTypeCidrBlock,
		NetworkEntityId: common.String(igwID),
	})
	_, err = c.vcn.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
		RtId:                    rtID,
		UpdateRouteTableDetails: core.UpdateRouteTableDetails{RouteRules: rules},
	})
	return err
}

func (c *Client) createDefaultVCNWithSubnet(ctx context.Context, compartmentID string) (*core.Subnet, error) {
	vcnResp, err := c.vcn.CreateVcn(ctx, core.CreateVcnRequest{
		CreateVcnDetails: core.CreateVcnDetails{
			CompartmentId: common.String(compartmentID),
			CidrBlocks:    []string{"10.0.0.0/16"},
			DisplayName:   common.String("oci-helper-vcn"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create vcn: %w", err)
	}
	vcnID := vcnResp.Vcn.Id
	if vcnID == nil {
		return nil, fmt.Errorf("vcn created without id")
	}
	for i := 0; i < 30; i++ {
		v, err := c.vcn.GetVcn(ctx, core.GetVcnRequest{VcnId: vcnID})
		if err == nil && v.Vcn.LifecycleState == core.VcnLifecycleStateAvailable {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	if err := c.ensureInternetGateway(ctx, *vcnID); err != nil {
		return nil, fmt.Errorf("internet gateway: %w", err)
	}
	subResp, err := c.vcn.CreateSubnet(ctx, core.CreateSubnetRequest{
		CreateSubnetDetails: core.CreateSubnetDetails{
			CompartmentId:          common.String(compartmentID),
			VcnId:                  vcnID,
			CidrBlock:              common.String("10.0.0.0/24"),
			DisplayName:            common.String("oci-helper-subnet"),
			ProhibitInternetIngress: common.Bool(false),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create subnet: %w", err)
	}
	if subResp.Subnet.Id == nil {
		return nil, fmt.Errorf("subnet created without id")
	}
	return &subResp.Subnet, nil
}

// LaunchTaskInstance launches an instance for a recurring create task using
// explicit resource sizing plus cloud-init root-password setup. The root
// password is also stored in a freeform tag, mirroring the Java original.
func (c *Client) LaunchTaskInstance(ctx context.Context, ad, shape, imageID, subnetID, displayName string,
	ocpus, memoryGB float32, diskGB int64, rootPassword string) (*core.Instance, error) {
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(ad),
		CompartmentId:      common.String(c.tenant.TenancyOCID),
		Shape:              common.String(shape),
		DisplayName:        common.String(displayName),
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(imageID),
			BootVolumeSizeInGBs: common.Int64(diskGB),
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId: common.String(subnetID),
		},
		AgentConfig: &core.LaunchInstanceAgentConfigDetails{
			IsMonitoringDisabled: common.Bool(true),
		},
	}
	if ocpus > 0 || memoryGB > 0 {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{}
		if ocpus > 0 {
			details.ShapeConfig.Ocpus = common.Float32(ocpus)
		}
		if memoryGB > 0 {
			details.ShapeConfig.MemoryInGBs = common.Float32(memoryGB)
		}
	}
	if rootPassword != "" {
		details.Metadata = map[string]string{
			"user_data": base64.StdEncoding.EncodeToString([]byte(BuildCloudInit(rootPassword))),
		}
		details.FreeformTags = map[string]string{"root-password": rootPassword}
	}
	resp, err := c.compute.LaunchInstance(ctx, core.LaunchInstanceRequest{LaunchInstanceDetails: details})
	if err != nil {
		return nil, err
	}
	return &resp.Instance, nil
}

// BuildCloudInit returns a cloud-init script that sets the root password and
// enables SSH password authentication. It mirrors the Java helper used by the
// original project.
func BuildCloudInit(password string) string {
	sanitized := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == 0 {
			return -1
		}
		if r < 32 {
			return -1
		}
		return r
	}, password)
	quotedPwd := "'" + strings.ReplaceAll(sanitized, "'", `'\''`) + "'"
	return "#cloud-config\n" +
		"ssh_pwauth: yes\n" +
		"chpasswd:\n" +
		"  list: |\n" +
		"    root:" + quotedPwd + "\n" +
		"  expire: false\n" +
		"write_files:\n" +
		"  - path: /tmp/setup_root_access.sh\n" +
		"    permissions: '0700'\n" +
		"    content: |\n" +
		"      #!/bin/bash\n" +
		"      if [ -f /etc/os-release ]; then\n" +
		"        . /etc/os-release\n" +
		"        OS=$ID\n" +
		"      else\n" +
		"        exit 1\n" +
		"      fi\n" +
		"      OS=$(echo \"$OS\" | tr '[:upper:]' '[:lower:]')\n" +
		"      sed -i 's/^#\\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config\n" +
		"      sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config\n" +
		"      if grep -q '^#\\?PrintMotd' /etc/ssh/sshd_config; then\n" +
		"        sed -i 's/^#\\?PrintMotd.*/PrintMotd no/' /etc/ssh/sshd_config\n" +
		"      else\n" +
		"        echo 'PrintMotd no' >> /etc/ssh/sshd_config\n" +
		"      fi\n" +
		"      if grep -q '^#\\?PrintLastLog' /etc/ssh/sshd_config; then\n" +
		"        sed -i 's/^#\\?PrintLastLog.*/PrintLastLog no/' /etc/ssh/sshd_config\n" +
		"      else\n" +
		"        echo 'PrintLastLog no' >> /etc/ssh/sshd_config\n" +
		"      fi\n" +
		"      case $OS in\n" +
		"        ubuntu|debian)\n" +
		"          if grep -q '^#\\?DenyUsers' /etc/ssh/sshd_config; then\n" +
		"            sed -i 's/^#\\?DenyUsers.*/DenyUsers ubuntu/' /etc/ssh/sshd_config\n" +
		"          else\n" +
		"            echo 'DenyUsers ubuntu' >> /etc/ssh/sshd_config\n" +
		"          fi\n" +
		"          ;;\n" +
		"        ol|rhel|centos|almalinux|rocky)\n" +
		"          if grep -q '^#\\?DenyUsers' /etc/ssh/sshd_config; then\n" +
		"            sed -i 's/^#\\?DenyUsers.*/DenyUsers opc/' /etc/ssh/sshd_config\n" +
		"          else\n" +
		"            echo 'DenyUsers opc' >> /etc/ssh/sshd_config\n" +
		"          fi\n" +
		"          ;;\n" +
		"      esac\n" +
		"      if command -v systemctl >/dev/null 2>&1; then\n" +
		"        systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null || true\n" +
		"      else\n" +
		"        service sshd restart 2>/dev/null || service ssh restart 2>/dev/null || true\n" +
		"      fi\n" +
		"runcmd:\n" +
		"  - [ bash, /tmp/setup_root_access.sh ]\n" +
		"  - echo 'Welcome to oci-helper managed instance' > /etc/motd\n" +
		"  - rm -f /tmp/setup_root_access.sh\n"
}

// UpdateRootPasswordTag sets or removes the "root-password" freeform tag on an
// instance without clobbering other tags.
func (c *Client) UpdateRootPasswordTag(ctx context.Context, instanceID, password string) error {
	inst, err := c.GetInstance(ctx, instanceID)
	if err != nil {
		return err
	}
	tags := map[string]string{}
	if inst.FreeformTags != nil {
		for k, v := range inst.FreeformTags {
			tags[k] = v
		}
	}
	if password == "" {
		delete(tags, "root-password")
	} else {
		tags["root-password"] = password
	}
	return c.UpdateInstanceFreeformTags(ctx, instanceID, tags)
}

// CreateBootVolumeBackup creates a full backup of a boot volume.
func (c *Client) CreateBootVolumeBackup(ctx context.Context, bootVolumeID, displayName string) (*core.BootVolumeBackup, error) {
	resp, err := c.bootVolume.CreateBootVolumeBackup(ctx, core.CreateBootVolumeBackupRequest{
		CreateBootVolumeBackupDetails: core.CreateBootVolumeBackupDetails{
			BootVolumeId: common.String(bootVolumeID),
			DisplayName:  common.String(displayName),
			Type:         core.CreateBootVolumeBackupDetailsTypeFull,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create boot volume backup: %w", err)
	}
	return &resp.BootVolumeBackup, nil
}

// GetBootVolumeBackup fetches a boot volume backup.
func (c *Client) GetBootVolumeBackup(ctx context.Context, backupID string) (*core.BootVolumeBackup, error) {
	resp, err := c.bootVolume.GetBootVolumeBackup(ctx, core.GetBootVolumeBackupRequest{
		BootVolumeBackupId: common.String(backupID),
	})
	if err != nil {
		return nil, fmt.Errorf("get boot volume backup: %w", err)
	}
	return &resp.BootVolumeBackup, nil
}

// DeleteBootVolumeBackup deletes a boot volume backup.
func (c *Client) DeleteBootVolumeBackup(ctx context.Context, backupID string) error {
	_, err := c.bootVolume.DeleteBootVolumeBackup(ctx, core.DeleteBootVolumeBackupRequest{
		BootVolumeBackupId: common.String(backupID),
	})
	if err != nil {
		return fmt.Errorf("delete boot volume backup: %w", err)
	}
	return nil
}

// ListNatGateways lists NAT gateways in a compartment/VCN.
func (c *Client) ListNatGateways(ctx context.Context, compartmentID, vcnID string) ([]core.NatGateway, error) {
	req := core.ListNatGatewaysRequest{
		CompartmentId: common.String(compartmentID),
	}
	if vcnID != "" {
		req.VcnId = common.String(vcnID)
	}
	resp, err := c.vcn.ListNatGateways(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// DeleteNatGateway deletes a NAT gateway.
func (c *Client) DeleteNatGateway(ctx context.Context, natGatewayID string) error {
	_, err := c.vcn.DeleteNatGateway(ctx, core.DeleteNatGatewayRequest{
		NatGatewayId: common.String(natGatewayID),
	})
	return err
}

// ListRouteTables lists route tables in a compartment/VCN.
func (c *Client) ListRouteTables(ctx context.Context, compartmentID, vcnID string) ([]core.RouteTable, error) {
	req := core.ListRouteTablesRequest{
		CompartmentId: common.String(compartmentID),
	}
	if vcnID != "" {
		req.VcnId = common.String(vcnID)
	}
	resp, err := c.vcn.ListRouteTables(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// DeleteRouteTable deletes a route table.
func (c *Client) DeleteRouteTable(ctx context.Context, routeTableID string) error {
	_, err := c.vcn.DeleteRouteTable(ctx, core.DeleteRouteTableRequest{
		RtId: common.String(routeTableID),
	})
	return err
}

// ClearNatGatewayCleanup removes NAT route tables and NAT gateways that were
// created for the 500Mbps feature. Route tables still in use by a VNIC are
// skipped to avoid breaking the tenant network.
func (c *Client) ClearNatGatewayCleanup(ctx context.Context, compartmentID, vcnID string) error {
	routeTables, err := c.ListRouteTables(ctx, compartmentID, vcnID)
	if err != nil {
		return err
	}
	for _, rt := range routeTables {
		if rt.Id == nil || rt.DisplayName == nil || !strings.Contains(*rt.DisplayName, "nat") {
			continue
		}
		for _, r := range rt.RouteRules {
			if r.Destination != nil && *r.Destination == "0.0.0.0/0" {
				_ = c.DeleteRouteTable(ctx, *rt.Id)
				break
			}
		}
	}
	natGWs, err := c.ListNatGateways(ctx, compartmentID, vcnID)
	if err != nil {
		return err
	}
	for _, ngw := range natGWs {
		if ngw.Id != nil {
			_ = c.DeleteNatGateway(ctx, *ngw.Id)
		}
	}
	return nil
}

// NLBInfo is a lightweight network load balancer summary used by tenant
// detail views.
type NLBInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	PublicIP string `json:"publicIp,omitempty"`
}

// ListNLBs lists network load balancers in a compartment.
func (c *Client) ListNLBs(ctx context.Context, compartmentID string) ([]NLBInfo, error) {
	resp, err := c.nlb.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
	})
	if err != nil {
		return nil, err
	}
	out := make([]NLBInfo, 0, len(resp.Items))
	for _, n := range resp.Items {
		info := NLBInfo{
			ID:     orEmpty(n.Id),
			Name:   orEmpty(n.DisplayName),
			Status: string(n.LifecycleState),
		}
		info.PublicIP = nlbPublicIP(n.IpAddresses)
		out = append(out, info)
	}
	return out, nil
}

func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SecurityRuleOptions carries the full set of rule attributes the Java panel
// supports: ICMP type/code, source/destination port ranges, stateless, and an
// optional description.
type SecurityRuleOptions struct {
	Protocol     string
	Port         string
	SourceOrDest string
	SourcePort   string
	ICMPType     int
	ICMPCode     int
	Stateless    bool
	Description  string
}

func (c *Client) AddIngressRuleAdvanced(ctx context.Context, vcnID string, opts SecurityRuleOptions) error {
	rule := core.IngressSecurityRule{
		Protocol:     common.String(opts.Protocol),
		Source:       common.String(opts.SourceOrDest),
		IsStateless:  common.Bool(opts.Stateless),
		Description:  stringPtrOrNil(opts.Description),
		SourceType:   core.IngressSecurityRuleSourceTypeCidrBlock,
	}
	if opts.Protocol == "1" || opts.Protocol == "58" {
		rule.IcmpOptions = &core.IcmpOptions{}
		if opts.ICMPType > 0 {
			rule.IcmpOptions.Type = common.Int(opts.ICMPType)
		}
		if opts.ICMPCode > 0 {
			rule.IcmpOptions.Code = common.Int(opts.ICMPCode)
		}
	} else if opts.Protocol == "TCP" || opts.Protocol == "UDP" {
		dest := parsePortRange(opts.Port)
		src := parsePortRange(opts.SourcePort)
		if opts.Protocol == "UDP" {
			rule.UdpOptions = &core.UdpOptions{}
			if dest != nil {
				rule.UdpOptions.DestinationPortRange = dest
			}
			if src != nil {
				rule.UdpOptions.SourcePortRange = src
			}
		} else {
			rule.TcpOptions = &core.TcpOptions{}
			if dest != nil {
				rule.TcpOptions.DestinationPortRange = dest
			}
			if src != nil {
				rule.TcpOptions.SourcePortRange = src
			}
		}
	}
	return c.applyIngressRule(ctx, vcnID, rule)
}

func (c *Client) AddEgressRuleAdvanced(ctx context.Context, vcnID string, opts SecurityRuleOptions) error {
	rule := core.EgressSecurityRule{
		Protocol:      common.String(opts.Protocol),
		Destination:   common.String(opts.SourceOrDest),
		IsStateless:   common.Bool(opts.Stateless),
		Description:   stringPtrOrNil(opts.Description),
		DestinationType: core.EgressSecurityRuleDestinationTypeCidrBlock,
	}
	if opts.Protocol == "1" || opts.Protocol == "58" {
		rule.IcmpOptions = &core.IcmpOptions{}
		if opts.ICMPType > 0 {
			rule.IcmpOptions.Type = common.Int(opts.ICMPType)
		}
		if opts.ICMPCode > 0 {
			rule.IcmpOptions.Code = common.Int(opts.ICMPCode)
		}
	} else if opts.Protocol == "TCP" || opts.Protocol == "UDP" {
		dest := parsePortRange(opts.Port)
		src := parsePortRange(opts.SourcePort)
		if opts.Protocol == "UDP" {
			rule.UdpOptions = &core.UdpOptions{}
			if dest != nil {
				rule.UdpOptions.DestinationPortRange = dest
			}
			if src != nil {
				rule.UdpOptions.SourcePortRange = src
			}
		} else {
			rule.TcpOptions = &core.TcpOptions{}
			if dest != nil {
				rule.TcpOptions.DestinationPortRange = dest
			}
			if src != nil {
				rule.TcpOptions.SourcePortRange = src
			}
		}
	}
	return c.applyEgressRule(ctx, vcnID, rule)
}

func (c *Client) applyIngressRule(ctx context.Context, vcnID string, newRule core.IngressSecurityRule) error {
	var page *string
	for {
		req := core.ListSecurityListsRequest{
			CompartmentId: common.String(c.tenant.TenancyOCID),
			VcnId:         common.String(vcnID),
			Limit:         common.Int(100),
			Page:          page,
		}
		resp, err := c.vcn.ListSecurityLists(ctx, req)
		if err != nil {
			return err
		}
		for _, sl := range resp.Items {
			ingressRules := sl.IngressSecurityRules
			if !hasIngressRule(ingressRules, newRule) {
				ingressRules = append(ingressRules, newRule)
			}
			if _, err := c.vcn.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
				SecurityListId: sl.Id,
				UpdateSecurityListDetails: core.UpdateSecurityListDetails{
					IngressSecurityRules: ingressRules,
					EgressSecurityRules:  sl.EgressSecurityRules,
				},
			}); err != nil {
				return fmt.Errorf("update security list %s: %w", *sl.Id, err)
			}
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return nil
}

func (c *Client) applyEgressRule(ctx context.Context, vcnID string, newRule core.EgressSecurityRule) error {
	var page *string
	for {
		req := core.ListSecurityListsRequest{
			CompartmentId: common.String(c.tenant.TenancyOCID),
			VcnId:         common.String(vcnID),
			Limit:         common.Int(100),
			Page:          page,
		}
		resp, err := c.vcn.ListSecurityLists(ctx, req)
		if err != nil {
			return err
		}
		for _, sl := range resp.Items {
			egressRules := sl.EgressSecurityRules
			if !hasEgressRule(egressRules, newRule) {
				egressRules = append(egressRules, newRule)
			}
			if _, err := c.vcn.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
				SecurityListId: sl.Id,
				UpdateSecurityListDetails: core.UpdateSecurityListDetails{
					IngressSecurityRules: sl.IngressSecurityRules,
					EgressSecurityRules:  egressRules,
				},
			}); err != nil {
				return fmt.Errorf("update security list %s: %w", *sl.Id, err)
			}
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return nil
}

func parsePortRange(s string) *core.PortRange {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.SplitN(s, "-", 2)
	min, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil
	}
	max := min
	if len(parts) > 1 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			max = v
		}
	}
	return &core.PortRange{Min: common.Int(min), Max: common.Int(max)}
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return common.String(s)
}
