/*Package xpo provides tooling to connect to the XPO Logistics API.  This is for truck shipments,
not small parcels.  Think LTL (less than truckload) shipments.  This code was created off the UPS API
documentation.  This uses UPS's JSON API.

Currently this package can perform:
- pickup requests

To create a pickup request:
- Set test or production mode (SetProductionMode()).
- Set your shipper (Shipper{}) and requestor (Requestor{}) info.
- Set shipment details (PkupItem{}).
- Request the pickup (RequestPickup()).
- Check for any errors.
*/
package xpo

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

//api urls
const (
	xpoTestURL       = "https://api.ltl.xpo.com/1.0/cust-pickup-requests"
	xpoProductionURL = "https://api.ltl.xpo.com/1.0/cust-pickup-requests?testMode=Y"
)

//xpoURL is se to the test URL by default
//This is changed to the production URL when the SetProductionMode function is called
//Forcing the developer to call the SetProductionMode function ensures the production URL is only used
//when actually needed.
var xpoURL = xpoTestURL

//timeout is the default time we should wait for a reply from XPO
//You may need to adjust this based on how slow connecting to XPO is for you.
//10 seconds is overly long, but sometimes XPO is very slow.
var timeout = time.Duration(10 * time.Second)

//role codes for what the requestor of the pickup is in relation to this shipment
var (
	RoleShipper    = "S"
	RoleConsignee  = "C"
	RoleThirdParty = "3"
)

//PickupRequest is the main container struct for data sent to XPO to request a pickup
//This is a single field with another container struct inside.  Why...who knows, ask XPO.
type PickupRequest struct {
	PickupRqstInfo PickupRqstInfo `json:"pickupRqstInfo"`
}

//PickupRqstInfo holds all the data on a pickup request
type PickupRqstInfo struct {
	//required
	PkupDate  string     `json:"pkupDate"`  //YYYY-MM-DDTHH:MM:SS
	ReadyTime string     `json:"readyTime"` //YYYY-MM-DDTHH:MM:SS
	CloseTime string     `json:"closeTime"` //YYYY-MM-DDTHH:MM:SS
	PkupItem  []PkupItem `json:"pkupItem"`  //items being picked up, up to 50

	//optional
	SpecialEquipmentCd string    `json:"specialEquipmentCd"`
	InsidePkupInd      bool      `json:"insidePkupInd"`
	Shipper            Shipper   `json:"shipper"`
	Requestor          Requestor `json:"requestor"`
	Contact            Contact   `json:"contact"` //usually same as requestor.contact
	Remarks            string    `json:"remarks"` //any random note
	TotPalletCnt       uint      `json:"totPalletCnt"`
	TotLoosePiecesCnt  uint      `json:"totLoosePiecesCnt"`
	TotWeight          Weight    `json:"totWeight"`
}

//Shipper holds data on the shipper
type Shipper struct {
	//required
	AddressLine1 string `json:"addressLine1"`
	CityName     string `json:"cityName"`
	StateCd      string `json:"stateCd"` //two character code

	//optional
	Name         string `json:"name"` //company name
	AddressLine2 string `json:"addressLine2"`
	PostalCd     string `json:"postalCd"`
	Phone        Phone  `json:"phone"`
}

//Requestor holds data on who requested the pickup
type Requestor struct {
	Contact Contact `json:"contact"`
	RoleCd  string  `json:"roleCd"` //"S" for shipper, "C" for consignee, "3" for third party
}

//Contact holds contact information
type Contact struct {
	CompanyName string `json:"companyName"`
	Email       Email  `json:"email"`
	FullName    string `json:"fullName"`
	Phone       Phone  `json:"phone"`
}

//Email holds an email address
//why this is a separate struct...ask XPO
type Email struct {
	EmailAddr string `json:"emailAddr"`
}

//Phone holds an phone number
//why this is a separate struct...ask XPO
type Phone struct {
	PhoneNbr string `json:"phoneNbr"`
}

//PkupItem is the good being picked up
type PkupItem struct {
	//required
	TotWeight Weight `json:"totWeight"`

	//optional
	DestZip6       string `json:"destZip6"` //ship to zip/postal code
	LoosePiecesCnt uint   `json:"loosePiecesCnt"`
	PalletCnt      uint   `json:"palletCnt"`
	GarntInd       bool   `json:"garntInd"` //guaranteed service
	HazmatInd      bool   `json:"hazmatInd"`
	FrzbleInd      bool   `json:"frzbleInd"`
	HolDlvrInd     bool   `json:"holDlvrInd"`   //holiday or weekend delivery requested
	FoodInd        bool   `json:"foodInd"`      //food stuffs
	BlkLiquidInd   bool   `json:"blkLiquidInd"` //bulk liquid shipment greater than 119 US gallons
	Remarks        string `json:"remarks"`      //random note for this pickup
}

//Weight holds a weight
type Weight struct {
	Weight float64 `json:"weight"`
}

//SuccessfulPickupResponse is the data returned when a pickup is scheduled
type SuccessfulPickupResponse struct {
	Code                 string `json:"code"`
	TransactionTimestamp string `json:"transactionTimestamp"` //unix timestamp
	Data                 struct {
		ConfirmationNbr string `json:"confirmationNbr"` //pickup confirmation number
	} `json:"data"`
}

//ErrorPickupResponse is the data returned when a pickup cannot be scheduled
//XPO API takes in JSON but returns XML upon error
//each field starts with "am:" but that can be excluded from struct tags
type ErrorPickupResponse struct {
	XMLName     xml.Name `xml:"fault"`
	Code        string   `xml:"code"`
	Type        string   `xml:"type"`
	Message     string   `xml:"message"`
	Description string   `xml:"description"`
}

//SetProductionMode chooses the production url for use
func SetProductionMode(yes bool) {
	if yes {
		xpoURL = xpoProductionURL
	}
	return
}

//SetTimeout updates the timeout value to something the user sets
//use this to increase the timeout if connecting to UPS is really slow
func SetTimeout(seconds time.Duration) {
	timeout = time.Duration(seconds * time.Second)
	return
}

//RequestPickup performs the API call to schedule a pickup
func (pri *PickupRqstInfo) RequestPickup() (response SuccessfulPickupResponse, err error) {
	//add the pickup request info to the pickup container object
	pr := PickupRequest{
		PickupRqstInfo: *pri,
	}

	//convert struct to json
	jsonBytes, err := json.Marshal(pr)
	if err != nil {
		err = errors.Wrap(err, "xpo.RequestPickup - could not marshal json")
		return
	}

	//make the call to XPO
	httpClient := http.Client{
		Timeout: timeout,
	}
	res, err := httpClient.Post(xpoURL, "application/json", bytes.NewReader(jsonBytes))
	if err != nil {
		err = errors.Wrap(err, "xpo.RequestPickup - could not make post request")
		return
	}

	//read the response
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		err = errors.Wrap(err, "xpo.RequestPickup - could not read response")
		return
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		//data might not be json, might be xml error
		//try unmarshaling to error xml
		var errorData ErrorPickupResponse
		err = xml.Unmarshal(body, &errorData)
		if err != nil {
			err = errors.Wrap(err, "xpo.RequestPickup - could not unmarshal response")
			return
		}

		//return our error so we know where this error came from, and UPS error message so we know what to fix
		log.Printf("%+v", errorData)
		err = errors.New(errorData.Description)
		return
	}

	//check if data was returned meaning request was successful
	//if not, reread the response data and log it
	if response.Data.ConfirmationNbr == "" {
		log.Println("xpo.RequestPickup - pickup request failed")
		log.Println(string(body))

		var errorData ErrorPickupResponse
		xml.Unmarshal(body, &errorData)

		//return our error so we know where this error came from, and UPS error message so we know what to fix
		err = errors.New("xpo.RequestPickup - pickup request failed")
		log.Println(errorData)
		return
	}

	//pickup request successful
	//response data will have confirmation number
	//an email should also have been sent to the requester email
	return
}
