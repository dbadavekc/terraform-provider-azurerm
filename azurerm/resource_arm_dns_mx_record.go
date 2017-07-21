package azurerm

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/arm/dns"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceArmDnsMxRecord() *schema.Resource {
	return &schema.Resource{
		Create: resourceArmDnsMxRecordCreateOrUpdate,
		Read:   resourceArmDnsMxRecordRead,
		Update: resourceArmDnsMxRecordCreateOrUpdate,
		Delete: resourceArmDnsMxRecordDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"resource_group_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"zone_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
			},

			"record": &schema.Schema{
				Type:     schema.TypeSet,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"preference": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},

						"exchange": &schema.Schema{
							Type:     schema.TypeString,
							Required: true,
						},
					},
				},
				Set: resourceArmDnsMxRecordHash,
			},

			"ttl": &schema.Schema{
				Type:     schema.TypeInt,
				Required: true,
			},

			"etag": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"tags": tagsSchema(),
		},
	}
}

func resourceArmDnsMxRecordCreateOrUpdate(d *schema.ResourceData, meta interface{}) error {
	dnsClient := meta.(*ArmClient).dnsClient

	name := d.Get("name").(string)
	resGroup := d.Get("resource_group_name").(string)
	zoneName := d.Get("zone_name").(string)
	ttl := int64(d.Get("ttl").(int))
	eTag := d.Get("etag").(string)

	tags := d.Get("tags").(map[string]interface{})
	metadata := expandTags(tags)

	records, err := expandAzureRmDnsMxRecords(d)
	props := dns.RecordSetProperties{
		Metadata:  metadata,
		TTL:       &ttl,
		MxRecords: &records,
	}

	parameters := dns.RecordSet{
		Name:                &name,
		RecordSetProperties: &props,
	}

	//last parameter is set to empty to allow updates to records after creation
	// (per SDK, set it to '*' to prevent updates, all other values are ignored)
	resp, err := dnsClient.CreateOrUpdate(resGroup, zoneName, name, dns.MX, parameters, eTag, "")
	if err != nil {
		return err
	}

	if resp.ID == nil {
		return fmt.Errorf("Cannot read DNS MX Record %s (resource group %s) ID", name, resGroup)
	}

	d.SetId(*resp.ID)

	return resourceArmDnsMxRecordRead(d, meta)
}

func resourceArmDnsMxRecordRead(d *schema.ResourceData, meta interface{}) error {
	dnsClient := meta.(*ArmClient).dnsClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	name := id.Path["MX"]
	zoneName := id.Path["dnszones"]

	resp, err := dnsClient.Get(resGroup, zoneName, name, dns.MX)
	if err != nil {
		return fmt.Errorf("Error reading DNS MX record %s: %v", name, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		d.SetId("")
		return nil
	}

	d.Set("name", name)
	d.Set("resource_group_name", resGroup)
	d.Set("zone_name", zoneName)
	d.Set("ttl", resp.TTL)
	d.Set("etag", resp.Etag)

	if err := d.Set("record", flattenAzureRmDnsMxRecords(resp.MxRecords)); err != nil {
		return err
	}
	flattenAndSetTags(d, resp.Metadata)

	return nil
}

func resourceArmDnsMxRecordDelete(d *schema.ResourceData, meta interface{}) error {
	dnsClient := meta.(*ArmClient).dnsClient

	id, err := parseAzureResourceID(d.Id())
	if err != nil {
		return err
	}

	resGroup := id.ResourceGroup
	name := id.Path["MX"]
	zoneName := id.Path["dnszones"]

	resp, error := dnsClient.Delete(resGroup, zoneName, name, dns.MX, "")
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Error deleting DNS MX Record %s: %s", name, error)
	}

	return nil
}

// flatten creates an array of map where preference is a string to suit
// the expectations of the ResourceData schema, so that this data can be
// managed by Terradata state.
func flattenAzureRmDnsMxRecords(records *[]dns.MxRecord) []map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(*records))

	if records != nil {
		for _, record := range *records {
			//results = append(results, *record.Ptrdname)
			preferenceI32 := *record.Preference
			preference := strconv.Itoa(int(preferenceI32))
			results = append(results, map[string]interface{}{
				"preference": preference,
				"exchange":   *record.Exchange,
			})
		}
	}

	return results
}

// expand creates an array of dns.MxRecord, that is, the array needed
// by azure-sdk-for-go to manipulate azure resources, hence Preference
// is an int32
func expandAzureRmDnsMxRecords(d *schema.ResourceData) ([]dns.MxRecord, error) {
	recordStrings := d.Get("record").(*schema.Set).List()
	records := make([]dns.MxRecord, len(recordStrings))

	for i, v := range recordStrings {
		mxrecord := v.(map[string]interface{})
		preference := mxrecord["preference"].(string)
		i64, _ := strconv.ParseInt(preference, 10, 32)
		i32 := int32(i64)
		exchange := mxrecord["exchange"].(string)

		records[i] = dns.MxRecord{
			Preference: &i32,
			Exchange:   &exchange,
		}
	}

	return records, nil
}

func resourceArmDnsMxRecordHash(v interface{}) int {
	var buf bytes.Buffer
	m := v.(map[string]interface{})
	buf.WriteString(fmt.Sprintf("%s-", m["preference"].(string)))
	buf.WriteString(fmt.Sprintf("%s-", m["exchange"].(string)))

	return hashcode.String(buf.String())
}
