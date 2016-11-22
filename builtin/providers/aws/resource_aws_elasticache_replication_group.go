package aws

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/elasticache"
	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsElasticacheReplicationGroupCommon() map[string]*schema.Schema {

	resourceSchema := resourceAwsElastiCacheCommonSchema()

	resourceSchema["replication_group_id"] = &schema.Schema{
		Type:         schema.TypeString,
		Required:     true,
		ForceNew:     true,
		ValidateFunc: validateAwsElastiCacheReplicationGroupId,
	}

	resourceSchema["replication_group_description"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
	}

	resourceSchema["description"] = &schema.Schema{
		Type:     schema.TypeString,
		Optional: true,
	}

	resourceSchema["auto_minor_version_upgrade"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
	}

	resourceSchema["engine"].Required = false
	resourceSchema["engine"].Optional = true
	resourceSchema["engine"].Default = "redis"
	resourceSchema["engine"].ValidateFunc = validateAwsElastiCacheReplicationGroupEngine

	return resourceSchema
}

func resourceAwsElasticacheReplicationGroup() *schema.Resource {

	resourceSchema := resourceAwsElasticacheReplicationGroupCommon()

	resourceSchema["number_cache_clusters"] = &schema.Schema{
		Type:         schema.TypeInt,
		Optional:     true,
		ForceNew:     true,
		ValidateFunc: validateAwsElastiCacheReplicationGroupNumCacheClusters,
	}

	// legacy
	resourceSchema["num_cache_clusters"] = &schema.Schema{
		Type:         schema.TypeInt,
		Optional:     true,
		ForceNew:     true,
		ValidateFunc: validateAwsElastiCacheReplicationGroupNumCacheClusters,
	}

	resourceSchema["automatic_failover_enabled"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
	}

	// legacy
	resourceSchema["automatic_failover"] = &schema.Schema{
		Type:     schema.TypeBool,
		Optional: true,
		Default:  false,
	}

	resourceSchema["primary_endpoint_address"] = &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
	}

	// legacy
	resourceSchema["primary_endpoint"] = &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
	}

	resourceSchema["configuration_endpoint_address"] = &schema.Schema{
		Type:     schema.TypeString,
		Computed: true,
	}

	return &schema.Resource{
		Create: resourceAwsElasticacheReplicationGroupCreate,
		Read:   resourceAwsElasticacheReplicationGroupRead,
		Update: resourceAwsElasticacheReplicationGroupUpdate,
		Delete: resourceAwsElasticacheReplicationGroupDelete,

		Schema: resourceSchema,
	}
}

func resourceAwsElasticacheReplicationGroupCreateSetup(d *schema.ResourceData, meta interface{}) (*elasticache.CreateReplicationGroupInput, error) {

	var node_type *string
	var description *string
	var replication_group_id *string

	if v, ok := d.GetOk("node_type"); ok {
		node_type = aws.String(v.(string))
	} else if v, ok := d.GetOk("cache_node_type"); ok {
		node_type = aws.String(v.(string))
	}

	if *node_type == "" {
		return nil, fmt.Errorf("node_type is required")
	}

	if v, ok := d.GetOk("replication_group_id"); ok {
		replication_group_id = aws.String(v.(string))
	} else if v, ok := d.GetOk("cluster_name"); ok {
		replication_group_id = aws.String(v.(string))
	} else {
		return nil, fmt.Errorf("replication_group_id is required")
	}

	if v, ok := d.GetOk("replication_group_description"); ok {
		description = aws.String(v.(string))
	} else if v, ok := d.GetOk("description"); ok {
		description = aws.String(v.(string))
	} else {
		return nil, fmt.Errorf("replication_group_description is required")
	}

	tags := tagsFromMapEC(d.Get("tags").(map[string]interface{}))
	params := &elasticache.CreateReplicationGroupInput{
		ReplicationGroupId:          replication_group_id,
		ReplicationGroupDescription: description,
		AutoMinorVersionUpgrade:     aws.Bool(d.Get("auto_minor_version_upgrade").(bool)),
		CacheNodeType:               node_type,
		Engine:                      aws.String(d.Get("engine").(string)),
		Tags:                        tags,
	}

	if v, ok := d.GetOk("port"); ok && v.(int) != 0 {
		params.Port = aws.Int64(int64(v.(int))) // e.g) 11211
	} else {
		params.Port = aws.Int64(int64(6379))
	}

	if v, ok := d.GetOk("engine_version"); ok {
		params.EngineVersion = aws.String(v.(string))
	}

	if v, ok := d.GetOk("automatic_failover_enabled"); ok {
		params.AutomaticFailoverEnabled = aws.Bool(v.(bool))
	} else if v, ok := d.GetOk("automatic_failover"); ok {
		params.AutomaticFailoverEnabled = aws.Bool(v.(bool))
	}

	preferred_azs := d.Get("availability_zones").(*schema.Set).List()
	if len(preferred_azs) == 0 {
		preferred_azs = d.Get("preferred_cache_cluster_azs").(*schema.Set).List()
	}

	if len(preferred_azs) > 0 {
		azs := expandStringList(preferred_azs)
		params.PreferredCacheClusterAZs = azs
	}

	if v, ok := d.GetOk("parameter_group_name"); ok {
		params.CacheParameterGroupName = aws.String(v.(string))
	}

	if v, ok := d.GetOk("subnet_group_name"); ok {
		params.CacheSubnetGroupName = aws.String(v.(string))
	}

	security_group_names := d.Get("security_group_names").(*schema.Set).List()
	if len(security_group_names) > 0 {
		params.CacheSecurityGroupNames = expandStringList(security_group_names)
	}

	security_group_ids := d.Get("security_group_ids").(*schema.Set).List()
	if len(security_group_ids) > 0 {
		params.SecurityGroupIds = expandStringList(security_group_ids)
	}

	snaps := d.Get("snapshot_arns").(*schema.Set).List()
	if len(snaps) > 0 {
		params.SnapshotArns = expandStringList(snaps)
	}

	if v, ok := d.GetOk("maintenance_window"); ok {
		params.PreferredMaintenanceWindow = aws.String(v.(string))
	}

	if v, ok := d.GetOk("notification_topic_arn"); ok {
		params.NotificationTopicArn = aws.String(v.(string))
	}

	if v, ok := d.GetOk("snapshot_retention_limit"); ok {
		params.SnapshotRetentionLimit = aws.Int64(int64(v.(int)))
	}

	if v, ok := d.GetOk("snapshot_window"); ok {
		params.SnapshotWindow = aws.String(v.(string))
	}

	if v, ok := d.GetOk("snapshot_name"); ok {
		params.SnapshotName = aws.String(v.(string))
	}

	return params, nil
}

func resourceAwsElasticacheReplicationGroupCreateCommon(d *schema.ResourceData, meta interface{}, params *elasticache.CreateReplicationGroupInput) error {
	conn := meta.(*AWSClient).elasticacheconn

	resp, err := conn.CreateReplicationGroup(params)
	if err != nil {
		return fmt.Errorf("Error creating Elasticache Replication Group: %s", err)
	}

	d.SetId(*resp.ReplicationGroup.ReplicationGroupId)

	pending := []string{"creating", "modifying", "restoring"}
	stateConf := &resource.StateChangeConf{
		Pending:    pending,
		Target:     []string{"available"},
		Refresh:    cacheReplicationGroupStateRefreshFunc(conn, d.Id(), "available", pending),
		Timeout:    40 * time.Minute,
		MinTimeout: 10 * time.Second,
		Delay:      30 * time.Second,
	}

	log.Printf("[DEBUG] Waiting for state to become available: %v", d.Id())
	_, sterr := stateConf.WaitForState()
	if sterr != nil {
		return fmt.Errorf("Error waiting for elasticache replication group (%s) to be created: %s", d.Id(), sterr)
	}

	return resourceAwsElasticacheReplicationGroupRead(d, meta)
}

func resourceAwsElasticacheReplicationGroupCreate(d *schema.ResourceData, meta interface{}) error {
	var number_cache_clusters int64

	params, err := resourceAwsElasticacheReplicationGroupCreateSetup(d, meta)
	if err != nil {
		return err
	}

	if v, ok := d.GetOk("number_cache_clusters"); ok {
		number_cache_clusters = int64(v.(int))
	} else if v, ok := d.GetOk("num_cache_clusters"); ok {
		number_cache_clusters = int64(v.(int))
	}

	if number_cache_clusters != 0 {
		params.NumCacheClusters = aws.Int64(number_cache_clusters)
	}

	return resourceAwsElasticacheReplicationGroupCreateCommon(d, meta, params)
}

func resourceAwsElasticacheReplicationGroupRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).elasticacheconn
	req := &elasticache.DescribeReplicationGroupsInput{
		ReplicationGroupId: aws.String(d.Id()),
	}

	res, err := conn.DescribeReplicationGroups(req)
	if err != nil {
		if eccErr, ok := err.(awserr.Error); ok && eccErr.Code() == "ReplicationGroupNotFoundFault" {
			log.Printf("[WARN] Elasticache Replication Group (%s) not found", d.Id())
			d.SetId("")
			return nil
		}

		return err
	}

	var rgp *elasticache.ReplicationGroup
	for _, r := range res.ReplicationGroups {
		if *r.ReplicationGroupId == d.Id() {
			rgp = r
		}
	}

	if rgp == nil {
		log.Printf("[WARN] Replication Group (%s) not found", d.Id())
		d.SetId("")
		return nil
	}

	if *rgp.Status == "deleting" {
		log.Printf("[WARN] The Replication Group %q is currently in the `deleting` state", d.Id())
		d.SetId("")
		return nil
	}

	if rgp.AutomaticFailover != nil {
		switch strings.ToLower(*rgp.AutomaticFailover) {
		case "disabled", "disabling":
			d.Set("automatic_failover_enabled", false)
		case "enabled", "enabling":
			d.Set("automatic_failover_enabled", true)
		default:
			log.Printf("Unknown AutomaticFailover state %s", *rgp.AutomaticFailover)
		}
	}

	d.Set("replication_group_description", rgp.Description)
	d.Set("number_cache_clusters", len(rgp.MemberClusters))
	d.Set("replication_group_id", rgp.ReplicationGroupId)

	if rgp.NodeGroups != nil {
		cacheCluster := *rgp.NodeGroups[0].NodeGroupMembers[0]

		res, err := conn.DescribeCacheClusters(&elasticache.DescribeCacheClustersInput{
			CacheClusterId:    cacheCluster.CacheClusterId,
			ShowCacheNodeInfo: aws.Bool(true),
		})
		if err != nil {
			return err
		}

		if len(res.CacheClusters) == 0 {
			return nil
		}

		c := res.CacheClusters[0]
		d.Set("node_type", c.CacheNodeType)
		d.Set("engine", c.Engine)
		d.Set("engine_version", c.EngineVersion)
		d.Set("subnet_group_name", c.CacheSubnetGroupName)
		d.Set("security_group_names", flattenElastiCacheSecurityGroupNames(c.CacheSecurityGroups))
		d.Set("security_group_ids", flattenElastiCacheSecurityGroupIds(c.SecurityGroups))

		if c.CacheParameterGroup != nil {
			d.Set("parameter_group_name", c.CacheParameterGroup.CacheParameterGroupName)
		}

		d.Set("maintenance_window", c.PreferredMaintenanceWindow)
		d.Set("snapshot_window", rgp.SnapshotWindow)
		d.Set("snapshot_retention_limit", rgp.SnapshotRetentionLimit)

		if rgp.ConfigurationEndpoint != nil {
			d.Set("port", rgp.ConfigurationEndpoint.Port)
			d.Set("configuration_endpoint_address", rgp.ConfigurationEndpoint.Address)
		} else {
			d.Set("port", rgp.NodeGroups[0].PrimaryEndpoint.Port)
			d.Set("primary_endpoint_address", rgp.NodeGroups[0].PrimaryEndpoint.Address)
			d.Set("primary_endpoint", rgp.NodeGroups[0].PrimaryEndpoint.Address)
		}

		d.Set("auto_minor_version_upgrade", c.AutoMinorVersionUpgrade)
	}

	return nil
}

func resourceAwsElasticacheReplicationGroupUpdate(d *schema.ResourceData, meta interface{}) error {
	var description *string
	var replication_group_description *string

	conn := meta.(*AWSClient).elasticacheconn

	requestUpdate := false
	params := &elasticache.ModifyReplicationGroupInput{
		ApplyImmediately:   aws.Bool(d.Get("apply_immediately").(bool)),
		ReplicationGroupId: aws.String(d.Id()),
	}

	if d.HasChange("description") {
		description = aws.String(d.Get("description").(string))
	} else if d.HasChange("replication_group_description") {
		replication_group_description = aws.String(d.Get("replication_group_description").(string))
	}

	if replication_group_description != nil {
		params.ReplicationGroupDescription = replication_group_description
		requestUpdate = true
	} else if (description != nil && replication_group_description != nil && *description != *replication_group_description) ||
		// Legacy: Fallback to description
		description != nil {
		params.ReplicationGroupDescription = description
		requestUpdate = true
	}

	if d.HasChange("automatic_failover") {
		params.AutomaticFailoverEnabled = aws.Bool(d.Get("automatic_failover").(bool))
		requestUpdate = true
	} else if d.HasChange("automatic_failover_enabled") {
		params.AutomaticFailoverEnabled = aws.Bool(d.Get("automatic_failover_enabled").(bool))
		requestUpdate = true
	}

	if d.HasChange("auto_minor_version_upgrade") {
		params.AutoMinorVersionUpgrade = aws.Bool(d.Get("auto_minor_version_upgrade").(bool))
		requestUpdate = true
	}

	if d.HasChange("security_group_ids") {
		if attr := d.Get("security_group_ids").(*schema.Set); attr.Len() > 0 {
			params.SecurityGroupIds = expandStringList(attr.List())
			requestUpdate = true
		}
	}

	if d.HasChange("security_group_names") {
		if attr := d.Get("security_group_names").(*schema.Set); attr.Len() > 0 {
			params.CacheSecurityGroupNames = expandStringList(attr.List())
			requestUpdate = true
		}
	}

	if d.HasChange("preferred_maintenance_window") {
		params.PreferredMaintenanceWindow = aws.String(d.Get("preferred_maintenance_window").(string))
		requestUpdate = true
	}

	if d.HasChange("notification_topic_arn") {
		params.NotificationTopicArn = aws.String(d.Get("notification_topic_arn").(string))
		requestUpdate = true
	}

	if d.HasChange("parameter_group_name") {
		params.CacheParameterGroupName = aws.String(d.Get("parameter_group_name").(string))
		requestUpdate = true
	}

	if d.HasChange("engine_version") {
		params.EngineVersion = aws.String(d.Get("engine_version").(string))
		requestUpdate = true
	}

	if d.HasChange("snapshot_retention_limit") {
		params.SnapshotRetentionLimit = aws.Int64(int64(d.Get("snapshot_retention_limit").(int)))
		requestUpdate = true
	}

	if d.HasChange("snapshot_window") {
		params.SnapshotWindow = aws.String(d.Get("snapshot_window").(string))
		requestUpdate = true
	}

	if d.HasChange("cache_node_type") {
		params.CacheNodeType = aws.String(d.Get("cache_node_type").(string))
		requestUpdate = true
	} else if d.HasChange("node_type") {
		node_type := d.Get("node_type").(string)
		if node_type != "" {
			params.CacheNodeType = aws.String(node_type)
			requestUpdate = true
		}
	}

	if requestUpdate {
		_, err := conn.ModifyReplicationGroup(params)
		if err != nil {
			return fmt.Errorf("Error updating Elasticache replication group: %s", err)
		}

		pending := []string{"creating", "modifying", "snapshotting"}
		stateConf := &resource.StateChangeConf{
			Pending:    pending,
			Target:     []string{"available"},
			Refresh:    cacheReplicationGroupStateRefreshFunc(conn, d.Id(), "available", pending),
			Timeout:    40 * time.Minute,
			MinTimeout: 10 * time.Second,
			Delay:      30 * time.Second,
		}

		log.Printf("[DEBUG] Waiting for state to become available: %v", d.Id())
		_, sterr := stateConf.WaitForState()
		if sterr != nil {
			return fmt.Errorf("Error waiting for elasticache replication group (%s) to be created: %s", d.Id(), sterr)
		}
	}
	return resourceAwsElasticacheReplicationGroupRead(d, meta)
}

func resourceAwsElasticacheReplicationGroupDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).elasticacheconn

	req := &elasticache.DeleteReplicationGroupInput{
		ReplicationGroupId: aws.String(d.Id()),
	}

	_, err := conn.DeleteReplicationGroup(req)
	if err != nil {
		if ec2err, ok := err.(awserr.Error); ok && ec2err.Code() == "ReplicationGroupNotFoundFault" {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("Error deleting Elasticache replication group: %s", err)
	}

	log.Printf("[DEBUG] Waiting for deletion: %v", d.Id())
	stateConf := &resource.StateChangeConf{
		Pending:    []string{"creating", "available", "deleting"},
		Target:     []string{},
		Refresh:    cacheReplicationGroupStateRefreshFunc(conn, d.Id(), "", []string{}),
		Timeout:    40 * time.Minute,
		MinTimeout: 10 * time.Second,
		Delay:      30 * time.Second,
	}

	_, sterr := stateConf.WaitForState()
	if sterr != nil {
		return fmt.Errorf("Error waiting for replication group (%s) to delete: %s", d.Id(), sterr)
	}

	return nil
}

func cacheReplicationGroupStateRefreshFunc(conn *elasticache.ElastiCache, replicationGroupId, givenState string, pending []string) resource.StateRefreshFunc {
	return func() (interface{}, string, error) {
		resp, err := conn.DescribeReplicationGroups(&elasticache.DescribeReplicationGroupsInput{
			ReplicationGroupId: aws.String(replicationGroupId),
		})
		if err != nil {
			if eccErr, ok := err.(awserr.Error); ok && eccErr.Code() == "ReplicationGroupNotFoundFault" {
				log.Printf("[DEBUG] Replication Group Not Found")
				return nil, "", nil
			}

			log.Printf("[ERROR] cacheClusterReplicationGroupStateRefreshFunc: %s", err)
			return nil, "", err
		}

		if len(resp.ReplicationGroups) == 0 {
			return nil, "", fmt.Errorf("[WARN] Error: no Cache Replication Groups found for id (%s)", replicationGroupId)
		}

		var rg *elasticache.ReplicationGroup
		for _, replicationGroup := range resp.ReplicationGroups {
			if *replicationGroup.ReplicationGroupId == replicationGroupId {
				log.Printf("[DEBUG] Found matching ElastiCache Replication Group: %s", *replicationGroup.ReplicationGroupId)
				rg = replicationGroup
			}
		}

		if rg == nil {
			return nil, "", fmt.Errorf("[WARN] Error: no matching ElastiCache Replication Group for id (%s)", replicationGroupId)
		}

		log.Printf("[DEBUG] ElastiCache Replication Group (%s) status: %v", replicationGroupId, *rg.Status)

		// return the current state if it's in the pending array
		for _, p := range pending {
			log.Printf("[DEBUG] ElastiCache: checking pending state (%s) for Replication Group (%s), Replication Group status: %s", pending, replicationGroupId, *rg.Status)
			s := *rg.Status
			if p == s {
				log.Printf("[DEBUG] Return with status: %v", *rg.Status)
				return s, p, nil
			}
		}

		return rg, *rg.Status, nil
	}
}

func validateAwsElastiCacheReplicationGroupEngine(v interface{}, k string) (ws []string, errors []error) {
	if strings.ToLower(v.(string)) != "redis" {
		errors = append(errors, fmt.Errorf("The only acceptable Engine type when using Replication Groups is Redis"))
	}
	return
}

func validateAwsElastiCacheReplicationGroupId(v interface{}, k string) (ws []string, errors []error) {
	value := v.(string)
	if (len(value) < 1) || (len(value) > 16) {
		errors = append(errors, fmt.Errorf(
			"%q must contain from 1 to 16 alphanumeric characters or hyphens got %q", k, value))
	}
	if !regexp.MustCompile(`^[0-9a-zA-Z-]+$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"only alphanumeric characters and hyphens allowed in %q", k))
	}
	if !regexp.MustCompile(`^[a-z]`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"first character of %q must be a letter", k))
	}
	if regexp.MustCompile(`--`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"%q cannot contain two consecutive hyphens", k))
	}
	if regexp.MustCompile(`-$`).MatchString(value) {
		errors = append(errors, fmt.Errorf(
			"%q cannot end with a hyphen", k))
	}
	return
}

func validateAwsElastiCacheReplicationGroupNumCacheClusters(v interface{}, k string) (ws []string, es []error) {
	value := v.(int)
	if value < 1 || value > 5 {
		es = append(es, fmt.Errorf(
			"number_cache_clusters must be within 0 and 5."))
	}
	return
}
