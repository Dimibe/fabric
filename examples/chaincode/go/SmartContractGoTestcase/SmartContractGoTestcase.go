package main

import (
	"bytes"
	"encoding/json"
	"errors"

	"log"
	"math/big"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//**************************************************//
//              Global variables                    //
//**************************************************//

var orderstatus = [...]string{
	"Created",
	"Assigned",
	"Started",
	"Finished",
}

const participants = "participants"
const orders = "orders"
const money = "money_"

//**************************************************//
//            Method Names in INIT                  //
//**************************************************//

const createOrder = "CREATE_ORDER"
const createParticipant = "CREATE_PARTICIPANT"
const createRating = "CREATE_RATING"
const subscribe = "CARRIER_SUBSCRIBE"
const selectCarrier = "SELECT_CARRIER"
const finishOrder = "COMPLETE_ORDER"
const coins = "GIVE_COINS"

//**************************************************//
//                  Structs                         //
//**************************************************//

//OurChaincode is our main struct that has (and manages) everything
type OurChaincode struct {
	orderCount        int
	participantCount  int
	notificationCount int
}

type rating struct {
	From  string `json:"author,omitempty"`
	Stars int    `json:"stars,omitempty"`
	Text  string `json:"message,omitempty"`
}

type geoLocation struct {
	GpsNorth    float64 `json:"gpsnorth,omitempty"`
	GpsEast     float64 `json:"gpseast,omitempty"`
	Description string  `json:"description,omitempty"`
}

type participant struct {
	Adress          *geoLocation   `json:"address,omitempty"`
	Name            string         `json:"name,omitempty"`
	Ratings         []rating       `json:"ratings,omitempty"`
	AverageRate     float64        `json:"averagerate"`
	Email           string         `json:"email,omitempty"`
	Orders          []int64        `json:"orderids,omitempty"`
	Notifications   []notification `json:"notifications"`
	Transactions    []notification `json:"transactions"`
	Password        string         `json:"password"`
	RegisteredSince time.Time      `json:"registeredSince"`
}

type order struct {
	ID                     *big.Int     `json:"id,omitempty"`
	Owner                  string       `json:"owner,omitempty"`
	Carriers               []string     `json:"carriers,omitempty"`
	IsBidOffer             bool         `json:"bidoffer,omitempty"`
	Status                 int          `json:"status"`
	EndOfSubscribtionOrBid time.Time    `json:"timeleft,omitempty"`
	SelectedCarrier        string       `json:"selectedcarrier,omitempty"`
	Price                  int          `json:"price,omitempty"`
	StartLocation          *geoLocation `json:"startLocation,omitempty"`
	EndLocation            *geoLocation `json:"endLocation,omitempty"`
	Description            string       `json:"description,omitempty"`
	CreationTime           time.Time    `json:"creationTime,omitempty"`
	Content                string       `json:"content,omitempty"`
	ActualStart            time.Time    `json:"actualStart,omitempty"`
	ActualEnd              time.Time    `json:"actualEnd,omitempty"`
	PlannedStart           time.Time    `json:"plannedStart,omitempty"`
	PlannedEnd             time.Time    `json:"plannedEnd,omitempty"`
	FinishCode             string       `json:"finishCode"`
}

type notification struct {
	ID   *big.Int  `json:"id"`
	Time time.Time `json:"time"`
	Type string    `json:"type"`
	Args []string  `json:"args"`
}

//**************************************************//
//             Help Methods INIT                    //
//**************************************************//

func (o *OurChaincode) createParticipant(stub shim.ChaincodeStubInterface, gpsNorth float64, gpsEast float64, Name, Email, locationDescription, password string) error {
	if gpsNorth != 0 && gpsEast != 0 && Name != "" && Email != "" {
		p := participant{}
		p.Name = Name
		p.Ratings = make([]rating, 0, 10)
		p.Notifications = make([]notification, 0, 10)
		p.Transactions = make([]notification, 0, 10)
		Email = strings.ToLower(Email)
		p.Email = Email
		//set location to gps values
		p.Adress = &geoLocation{gpsNorth, gpsEast, locationDescription}
		p.Password = password
		p.RegisteredSince = time.Now()
		res, err := o.participantToJSON(stub, &p)
		if err != nil {
			return err
		}
		err = stub.PutState(participants+"_"+Email, res)
		if err != nil {
			log.Println(err.Error())
			return err
		}
		err = stub.PutState(money+participants+"_"+Email, []byte(strconv.Itoa(0)))
		if err != nil {
			log.Println(err.Error())
		}
		return err
	}
	log.Println("Invalid argument values")
	return errors.New("Invalid argument values")

}

func (o *OurChaincode) newOrder(stub shim.ChaincodeStubInterface, Email string, bid bool, price int, gpsNorthStart, gpsEastStart, gpsNorthEnd, gpsEastEnd float64, description, content, plannedStart, plannedEnd, timeToCarrierSelection, startLocationDescription, endLocationDescription string) ([]byte, error) {
	Email = strings.ToLower(Email)
	res, err := stub.GetState(participants + "_" + Email)
	var p participant
	if err != nil || len(res) == 0 {
		if len(res) == 0 {
			s := "Owner not found"
			log.Println(s)
			err = errors.New(s)
		} else {
			log.Println(err.Error())
		}
		return nil, err
	}
	err = json.Unmarshal(res, &p)
	if err != nil || p.Name == "" {
		if p.Name == "" {
			return nil, errors.New("Unmarshal did wrong")
		}
		log.Println(err.Error())
		return nil, err
	}
	or := order{}
	or.Owner = p.Email
	or.Carriers = make([]string, 0, 10)
	or.ID = o.getIDForOrder()
	or.IsBidOffer = bid
	or.Status = 0
	or.SelectedCarrier = ""
	or.Price = price
	or.StartLocation = &geoLocation{gpsNorthStart, gpsEastStart, startLocationDescription}
	or.EndLocation = &geoLocation{gpsNorthEnd, gpsEastEnd, endLocationDescription}
	or.Description = description
	or.CreationTime = time.Now()
	or.Content = content
	start, err := time.Parse("2006-01-02T15:04Z", plannedStart)
	if err != nil {
		s := "Error parsing start time in " + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}
	end, err := time.Parse("2006-01-02T15:04Z", plannedEnd)
	if err != nil {
		s := "Error parsing end time in " + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}
	if !end.After(start) {
		s := "Invalid start and end times in" + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}
	or.PlannedStart = start
	or.PlannedEnd = end

	selection, err := time.Parse("2006-01-02T15:04Z", timeToCarrierSelection)
	if err != nil {
		s := "Error parsing end time of selection in " + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}
	if !selection.Before(start) {
		s := "The selection must be before the earliest start of the order"
		log.Println(s)
		return nil, errors.New(s)
	}
	or.EndOfSubscribtionOrBid = selection

	//add to participant list of orderids
	if p.Orders == nil || len(p.Orders) == 0 || cap(p.Orders)+1 > len(p.Orders) {
		copy := make([]int64, len(p.Orders), (cap(p.Orders)+1)*2) //+1 in case cap(p.Orders) == 0
		for i := range p.Orders {
			copy[i] = p.Orders[i]
		}
		p.Orders = copy
	}
	p.Orders = append(p.Orders, or.ID.Int64())

	if len(p.Notifications) == 0 || cap(p.Notifications)+1 > len(p.Notifications) {
		copy := make([]notification, len(p.Notifications), (cap(p.Notifications)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Notifications {
			copy[i] = p.Notifications[i]
		}
		p.Notifications = copy
	}
	p.Notifications = append(p.Notifications, notification{o.getIDForNotification(), time.Now(), "ORDER_CREATED", []string{or.ID.String()}})

	part, err := o.participantToJSON(stub, &p)
	if err != nil {
		return nil, err
	}
	err = stub.PutState(participants+"_"+p.Email, part)
	if err != nil {
		return nil, err
	}

	//add to all Orders
	orderJSON, err := o.ordersToJSON(stub, &or, "", []int64{}, true)
	if err != nil {
		return nil, err
	}

	err = stub.PutState(orders+"_"+or.ID.String(), orderJSON)
	if err != nil {
		return nil, err
	}
	o.orderCount++

	return nil, nil
}

func (o *OurChaincode) createRating(stub shim.ChaincodeStubInterface, from, to string, stars int, text, orderid string) error {
	//pFrom, ok := o.allParticipants[from]

	orderbytes, err := stub.GetState(orders + "_" + orderid)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	var or order
	err = json.Unmarshal(orderbytes, &or)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	if or.Status != 3 {
		err = errors.New("Order in status " + string(or.Status) + " instead of 3")
		log.Println(err.Error())
		return err
	}
	or.Status = or.getNextStatus()
	orderbytes, err = o.ordersToJSON(stub, &or, "", []int64{}, true)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	err = stub.PutState(orders+"_"+orderid, orderbytes)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	from = strings.ToLower(from)
	to = strings.ToLower(to)
	pFromBytes, err := stub.GetState(participants + "_" + from)
	if err != nil {
		log.Println("From not found")
		return errors.New("From not found")
	}

	pToBytes, err := stub.GetState(participants + "_" + to)
	if err != nil {
		log.Println("To not found")
		return errors.New("To not found")
	}

	var pFrom, pTo participant
	err = json.Unmarshal(pFromBytes, &pFrom)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = json.Unmarshal(pToBytes, &pTo)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	//for remembering: len -> used space in the slice
	// cap -> length of the slice [1,0,0] -> len = 1, cap = 3
	if len(pTo.Ratings) == 0 || len(pTo.Ratings)+1 > cap(pTo.Ratings) {
		//make slice / array bigger
		copy := make([]rating, len(pTo.Ratings), (cap(pTo.Ratings)+1)*2) //+1 in case caps(p.Ratings) == 0
		for i := range pTo.Ratings {
			copy[i] = pTo.Ratings[i]
		}
		pTo.Ratings = copy
	}
	r := rating{pFrom.Email, stars, text}
	pTo.Ratings = append(pTo.Ratings, r)
	pTo.AverageRate = o.getAverage(pTo)

	if len(pTo.Notifications) == 0 || cap(pTo.Notifications)+1 > len(pTo.Notifications) {
		copy := make([]notification, len(pTo.Notifications), (cap(pTo.Notifications)+1)*2) //+1 in case cap(pTo.notifications) == 0
		for i := range pTo.Notifications {
			copy[i] = pTo.Notifications[i]
		}
		pTo.Notifications = copy
	}
	pTo.Notifications = append(pTo.Notifications, notification{o.getIDForNotification(), time.Now(), "RATING", []string{strconv.Itoa(stars)}})

	pToBytes, err = o.participantToJSON(stub, &pTo)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = stub.PutState(participants+"_"+pTo.Email, pToBytes)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	return nil
}

func (o *OurChaincode) subscribeCarrier(stub shim.ChaincodeStubInterface, carrier participant, order order) error {
	if time.Now().After(order.EndOfSubscribtionOrBid) {
		s := "Its to late to subscribe"
		log.Println(s)
		return errors.New(s)
	}

	copy, inserted := sortedInsert(order.Carriers, carrier.Email)
	order.Carriers = copy

	if inserted {
		ownerbytes, err := stub.GetState(participants + "_" + order.Owner)
		if err != nil {
			s := "Owner not found"
			log.Println(s)
			return errors.New(s)
		}
		var owner participant
		err = json.Unmarshal(ownerbytes, &owner)
		if err != nil {
			s := "Error unmarshaling owner"
			log.Println(s)
			return errors.New(s)
		}

		if len(owner.Notifications) == 0 || cap(owner.Notifications)+1 > len(owner.Notifications) {
			copy := make([]notification, len(owner.Notifications), (cap(owner.Notifications)+1)*2) //+1 in case cap(owner.notifications) == 0
			for i := range owner.Notifications {
				copy[i] = owner.Notifications[i]
			}
			owner.Notifications = copy
		}
		owner.Notifications = append(owner.Notifications, notification{o.getIDForNotification(), time.Now(), "CARRIER_SUBSCRIBED", []string{order.ID.String()}})

		ownerbytes, err = json.Marshal(owner)
		if err != nil {
			s := "Error marshalling owner"
			log.Println(s)
			return errors.New(s)
		}
		err = stub.PutState(participants+"_"+owner.Email, ownerbytes)
		if err != nil {
			s := "Error putting state of owner"
			log.Println(s)
			return errors.New(s)
		}
	}

	orderBytes, err := o.ordersToJSON(stub, &order, "", []int64{}, true)
	if err != nil {
		return err
	}
	err = stub.PutState(orders+"_"+order.ID.String(), orderBytes)
	if err != nil {
		s := "Error in order putstate"
		log.Println(s)
		return errors.New(s)
	}
	return nil
}

func sortedInsert(field []string, v string) ([]string, bool) {
	len := len(field)
	if len == 0 {
		return []string{v}, true
	}
	ret := make([]string, 0, cap(field))
	if field[0] > v {
		ret = append(ret, v)
		for i := 0; i < len; i++ {
			ret = append(ret, field[i])
		}
	} else if field[len-1] < v {
		for i := 0; i < len; i++ {
			ret = append(ret, field[i])
		}
		ret = append(ret, v)
	} else {
		found := false
		for i := 0; i < len+1; i++ {
			if found {
				ret = append(ret, field[i-1])
			} else if field[i] < v {
				ret = append(ret, field[i])
			} else if field[i] == v {
				return field, false
			} else {
				ret = append(ret, v)
				found = true
			}
		}
	}
	return ret, true
}

func (o *OurChaincode) selectCarrier(stub shim.ChaincodeStubInterface, car participant, order order) error {
	order.SelectedCarrier = car.Email
	order.Carriers = make([]string, 0, 0)
	order.Status = order.getNextStatus()

	owner, err := stub.GetState(money + participants + "_" + order.Owner)
	if err != nil {
		s := "GetState for owner failed in method " + finishOrder
		log.Println(s)
		return errors.New(s)
	}
	ownerMoney, err := strconv.ParseInt(string(owner), 10, 64)
	if err != nil {
		s := "Something went wrong parsing the Owner Money"
		log.Println(s)
		return errors.New(s)
	}
	if ownerMoney < int64(order.Price) {
		s := "Owner has not enough Money to pay the Carrier"
		log.Println(s)
		return errors.New(s)
	}
	ownerMoney = ownerMoney - int64(order.Price)
	err = stub.PutState(money+participants+"_"+order.Owner, []byte(strconv.Itoa(int(ownerMoney))))
	if err != nil {
		s := "something went wrong saving the balance"
		log.Println(s)
		return errors.New(s)
	}

	if len(car.Notifications) == 0 || cap(car.Notifications)+1 > len(car.Notifications) {
		copy := make([]notification, len(car.Notifications), (cap(car.Notifications)+1)*2) //+1 in case cap(car.notifications) == 0
		for i := range car.Notifications {
			copy[i] = car.Notifications[i]
		}
		car.Notifications = copy
	}
	car.Notifications = append(car.Notifications, notification{o.getIDForNotification(), time.Now(), "CARRIER_SELECTED", []string{order.ID.String()}})
	carBytes, err := json.Marshal(car)
	if err != nil {
		s := "Error marshalling carrier"
		log.Println(s)
		return errors.New(s)
	}

	err = stub.PutState(participants+"_"+car.Email, carBytes)
	if err != nil {
		s := "Error put State carrier"
		log.Println(s)
		return errors.New(s)
	}
	order.FinishCode = o.getRandomString(16)
	orderBytes, err := o.ordersToJSON(stub, &order, "", []int64{}, true)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = stub.PutState(orders+"_"+order.ID.String(), orderBytes)
	if err != nil {
		s := "Error while putting order state"
		log.Println(s)
		return err
	}

	pbytes, err := stub.GetState(participants + "_" + order.Owner)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	if len(p.Transactions) == 0 || cap(p.Transactions)+1 > len(p.Transactions) {
		copy := make([]notification, len(p.Transactions), (cap(p.Transactions)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Transactions {
			copy[i] = p.Transactions[i]
		}
		p.Transactions = copy
	}
	p.Transactions = append(p.Transactions, notification{o.getIDForNotification(), time.Now(), "COINS_OUT", []string{strconv.Itoa(order.Price)}})
	pbytes, err = json.Marshal(p)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = stub.PutState(participants+"_"+p.Email, pbytes)
	if err != nil {
		log.Println(err.Error())
	}

	return nil
}

func (p *participant) equals(p2 participant) bool {
	if p == nil || p.Name == "" || p2.Name == "" {
		return false
	}
	return strings.Compare(strings.ToUpper(p2.Name), strings.ToUpper(p.Name)) == 0
}

func (o *OurChaincode) completeOrder(stub shim.ChaincodeStubInterface, order order) error {
	order.Status = order.getNextStatus()
	_, err := stub.GetState(participants + "_" + order.SelectedCarrier)
	if err != nil {
		s := "GetState for carrier failed in method " + finishOrder
		log.Println(s)
		return errors.New(s)
	}

	carrierMoneyBytes, err := stub.GetState(money + participants + "_" + order.SelectedCarrier)
	if err != nil {
		s := "GetState for carrier money failed in method " + finishOrder
		log.Println(s)
		return errors.New(s)
	}
	carrierMoney, err := strconv.ParseInt(string(carrierMoneyBytes), 10, 64)
	if err != nil {
		s := "Something went wrong parsing the Money"
		log.Println(s)
		return errors.New(s)
	}
	carrierMoney = carrierMoney + int64(order.Price)
	err = stub.PutState(money+participants+"_"+order.SelectedCarrier, []byte(strconv.Itoa(int(carrierMoney))))
	if err != nil {
		s := "something went wrong saving the balance"
		log.Println(s)
		return errors.New(s)
	}

	if err != nil {
		s := "something went wrong saving the balance"
		log.Println(s)
		return errors.New(s)
	}
	order.ActualEnd = time.Now()

	var p participant
	pbytes, err := stub.GetState(participants + "_" + order.Owner)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	if len(p.Notifications) == 0 || cap(p.Notifications)+1 > len(p.Notifications) {
		copy := make([]notification, len(p.Notifications), (cap(p.Notifications)+1)*2) //+1 in case cap(order.Owner.notifications) == 0
		for i := range p.Notifications {
			copy[i] = p.Notifications[i]
		}
		p.Notifications = copy
	}
	p.Notifications = append(p.Notifications, notification{o.getIDForNotification(), time.Now(), "ORDER_FINISHED", []string{order.ID.String()}})
	ownerbytes, err := json.Marshal(p)
	if err != nil {
		s := "Error in marshaling owner"
		log.Println(s)
		return errors.New(s)
	}
	err = stub.PutState(participants+"_"+p.Email, ownerbytes)
	if err != nil {
		s := "Error in put state of owner"
		log.Println(s)
		return errors.New(s)
	}

	pbytes, err = stub.GetState(participants + "_" + order.SelectedCarrier)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	//COINS_IN
	if len(p.Transactions) == 0 || cap(p.Transactions)+1 > len(p.Transactions) {
		copy := make([]notification, len(p.Transactions), (cap(p.Transactions)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Transactions {
			copy[i] = p.Transactions[i]
		}
		p.Transactions = copy
	}
	p.Transactions = append(p.Transactions, notification{o.getIDForNotification(), time.Now(), "COINS_IN", []string{strconv.Itoa(order.Price)}})
	//ORDER_SUCCESS
	if len(p.Notifications) == 0 || cap(p.Notifications)+1 > len(p.Notifications) {
		copy := make([]notification, len(p.Notifications), (cap(p.Notifications)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Notifications {
			copy[i] = p.Notifications[i]
		}
		p.Notifications = copy
	}
	p.Notifications = append(p.Notifications, notification{o.getIDForNotification(), time.Now(), "ORDER_SUCCESS", []string{order.ID.String()}})

	pbytes, err = json.Marshal(p)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = stub.PutState(participants+"_"+p.Email, pbytes)
	if err != nil {
		log.Println(err.Error())
		return err
	}

	bytes, err := o.ordersToJSON(stub, &order, "", []int64{}, true)
	if err != nil {
		log.Println(err.Error())
		return err
	}
	err = stub.PutState(orders+"_"+order.ID.String(), bytes)
	if err != nil {
		log.Println(err.Error())
	}
	return err
}

func (o *OurChaincode) participantToJSON(stub shim.ChaincodeStubInterface, p *participant) ([]byte, error) {
	var result []byte
	result, err := json.Marshal(p)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}

	return result, nil
}

func (o *OurChaincode) deleteOrder(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		s := "Delete order only needs the orderid as param."
		log.Println(s)
		return nil, errors.New(s)
	}

	orderbytes, err := stub.GetState(orders + "_" + args[0])
	if len(orderbytes) == 0 || err != nil {
		err = errors.New("Order with id " + args[0] + " not found.")
		log.Println(err.Error())
		return nil, err
	}
	var or order
	err = json.Unmarshal(orderbytes, &or)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	if or.SelectedCarrier != "" {
		s := "Order has a selected Carrier"
		log.Println(s)
		return nil, errors.New(s)
	}
	err = stub.DelState(orders + "_" + args[0])
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	return nil, nil
}

//**************************************************//
//              Query  Functions                    //
//**************************************************//

func (o *OurChaincode) ordersToJSON(stub shim.ChaincodeStubInterface, or *order, status string, ids []int64, finishcode bool) ([]byte, error) {
	var result []byte
	var err error
	if or == nil {
		var list []order
		if len(ids) != 0 {
			list = make([]order, 0, len(ids))
		} else {
			list = make([]order, 0, o.orderCount)
		}
		emptylist, err := json.Marshal(list)
		for i := 0; i < o.orderCount; i++ {
			val, err := stub.GetState(orders + "_" + strconv.Itoa(i+1)) //order id faengt bei 1 an
			if err != nil {
				return emptylist, err
			}
			if len(val) == 0 {
				continue
			}
			var order order
			err = json.Unmarshal(val, &order)
			if err != nil {
				s := "Failed to unmarshal order in orderstojson"
				log.Println(s)
				return nil, errors.New(s)
			}
			if !finishcode {
				order.FinishCode = ""
			}

			var idfound = len(ids) == 0

			for j := range ids {
				if order.ID.Int64() == ids[j] {
					idfound = true
				}
				if idfound {
					break
				}
			}

			if idfound && (status == "" || strings.EqualFold(orderstatus[order.Status], status)) {
				list = append(list, order)
			}
		}

		result, err = json.Marshal(list)
		if err != nil {
			return nil, errors.New(err.Error())
		}
	} else {
		result, err = json.Marshal(or)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (o *OurChaincode) ordersByParticipant(stub shim.ChaincodeStubInterface, email string) ([]byte, error) {
	if email == "" {
		s := "No Email given"
		log.Println(s)
		return nil, errors.New(s)
	}
	email = strings.ToLower(email)
	orderlist := make([]order, 0, o.orderCount)
	for i := 0; i < o.orderCount; i++ {
		val, err := stub.GetState(orders + "_" + strconv.Itoa(i+1))
		if len(val) == 0 {
			continue
		}
		if err != nil {
			log.Println(err.Error() + " while id =" + strconv.Itoa(i+1) + " (order not found)")
			continue
		}
		var or order
		err = json.Unmarshal(val, &or)
		if err != nil {
			log.Println(err.Error() + " while id =" + strconv.Itoa(i+1) + " unmarshal went wrong")
			continue
		}
		if strings.Compare(or.Owner, email) == 0 {
			orderlist = append(orderlist, or)
		}
	}
	ret, err := json.Marshal(&orderlist)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	return ret, nil
}

func (o *OurChaincode) ordersByCarrier(stub shim.ChaincodeStubInterface, email string, status []int) ([]byte, error) {
	if email == "" {
		s := "No Email given"
		log.Println(s)
		return nil, errors.New(s)
	}
	email = strings.ToLower(email)
	allorders := make([]order, 0, o.orderCount)
	for j := 0; j < len(status); j++ {
		val, err := o.ordersToJSON(stub, nil, orderstatus[status[j]], []int64{}, false)
		if err != nil || len(val) == 0 {
			err = errors.New("No orders found in status " + orderstatus[status[j]])
			log.Println(err.Error())
			continue
		}

		var orders []order
		err = json.Unmarshal(val, &orders)
		if err != nil || len(val) == 0 {
			if err == nil {
				s := "Unmarshal went wrong"
				log.Println(s)
				return nil, errors.New(s)
			}
			log.Println(err.Error())
			return nil, err
		}
		for i := 0; i < len(orders); i++ {
			if strings.EqualFold(orders[i].SelectedCarrier, email) {
				allorders = append(allorders, orders[i])
			}
		}
	}
	return json.Marshal(&allorders)
}

func (o *OurChaincode) notificationsByParticipant(stub shim.ChaincodeStubInterface, email string) ([]byte, error) {
	pbytes, err := stub.GetState(participants + "_" + email)
	if len(pbytes) == 0 || err != nil {
		s := "Participant with mail " + email + " not found"
		log.Println(s)
		return nil, errors.New(s)
	}
	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		s := "Unmarshal failed"
		log.Println(s)
		return nil, errors.New(s)
	}
	res, err := json.Marshal(p.Notifications)
	if err != nil {
		s := "Marshaling notifications failed"
		log.Println(s)
		return nil, errors.New(s)
	}
	return res, nil
}

func (o *OurChaincode) transactionsByParticipant(stub shim.ChaincodeStubInterface, email string) ([]byte, error) {
	pbytes, err := stub.GetState(participants + "_" + email)
	if len(pbytes) == 0 || err != nil {
		s := "Participant with mail " + email + " not found"
		log.Println(s)
		return nil, errors.New(s)
	}
	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		s := "Unmarshal failed"
		log.Println(s)
		return nil, errors.New(s)
	}
	res, err := json.Marshal(p.Transactions)
	if err != nil {
		s := "Marshaling transactions failed"
		log.Println(s)
		return nil, errors.New(s)
	}
	return res, nil
}

func (o *order) getNextStatus() int {
	return o.Status + 1
}

func (o *OurChaincode) getIDForOrder() *big.Int {
	if o.orderCount == 0 {
		return big.NewInt(1)
	}
	return big.NewInt(int64(o.orderCount) + 1)
}

func (o *OurChaincode) getIDForParticipant() *big.Int {
	if o.participantCount == 0 {
		return big.NewInt(1)
	}
	return big.NewInt(int64(o.participantCount) + 1)
}

func (o *OurChaincode) getAverage(p participant) float64 {
	var average int //default 0
	if p.Name != "" && len(p.Ratings) > 0 {
		for i := 0; i < len(p.Ratings); i++ {
			average += p.Ratings[i].Stars
		}
		return float64(average) / float64(len(p.Ratings))
	}
	return 0
}

func (o *OurChaincode) getIDForNotification() *big.Int {
	o.notificationCount++
	return big.NewInt(int64(o.notificationCount))
}

func (o *OurChaincode) isRegistered(stub shim.ChaincodeStubInterface, email, password string) ([]byte, error) {
	email = strings.ToLower(email)
	val, err := stub.GetState(participants + "_" + email)
	if len(val) == 0 || err != nil {
		return []byte("false"), nil
	}
	var p participant
	err = json.Unmarshal(val, &p)
	if err != nil || p.Name == "" || (password != "" && bytes.Compare([]byte(p.Password), []byte(password)) != 0) {
		return []byte("false"), nil
	}
	return []byte("true"), nil
}

//**************************************************//
//      Methods called from Invoke / API            //
//**************************************************//

func (o *OurChaincode) handleOrderCreation(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//args: "Emailofshipper, isbidoffer, price, gpsNorthStart, gpsEastStart, gpsNorthEnd, gpsEastEnd, description, content,plannedStart, plannedEnd, carrierSelectionTime"
	if len(args) != 14 {
		s := "Invalid argument count for " + createOrder + " , args: Emailofshipper, isbidoffer, price, gpsNorthStart, gpsEastStart, gpsNorthEnd, gpsEastEnd, description, content,plannedStart, plannedEnd, carrierSelectionTime"
		return nil, errors.New(s)
	}
	bid, err := strconv.ParseBool(args[1])
	if err != nil {
		bid = false
		log.Println("Boolean value " + args[1] + " could not be read. Setting to false now.")
	}
	val, err := strconv.Atoi(args[2])
	if err != nil {
		s := "3rd value must be integer value"
		log.Println(s)
		return nil, errors.New(s)
	}
	gpsNorthStart, err := strconv.ParseFloat(args[3], 64)
	gpsEastStart, err2 := strconv.ParseFloat(args[4], 64)
	if err != nil || err2 != nil {
		s := "4th and 5th value must be float64 in method " + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}
	gpsNorthEnd, err := strconv.ParseFloat(args[5], 64)
	gpsEastEnd, err2 := strconv.ParseFloat(args[6], 64)
	if err != nil || err2 != nil {
		s := "6th and 7th value must be float64 in method " + createOrder
		log.Println(s)
		return nil, errors.New(s)
	}

	return o.newOrder(stub, args[0], bid, val, gpsNorthStart, gpsEastStart, gpsNorthEnd, gpsEastEnd, args[7], args[8], args[9], args[10], args[11], args[12], args[13])
}

func (o *OurChaincode) handleParticipantCreation(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 6 {
		s := "Function handleParticipantCreation expects 6 arguments, see documentation for help"
		log.Println(s)
		return nil, errors.New(s)
	}
	gpsNorth, err := strconv.ParseFloat(args[0], 64)
	gpsEast, err2 := strconv.ParseFloat(args[1], 64)
	if err != nil || err2 != nil {
		s := "The first an second value must be a float64 value"
		log.Println(s)
		return nil, errors.New(s)
	}

	val, err := stub.GetState(participants + "_" + strings.ToLower(args[3]))
	if err != nil || len(val) != 0 {
		s := "Participant with this Email is already created!"
		log.Println(s)
		return nil, errors.New(s)
	}
	return nil, o.createParticipant(stub, gpsNorth, gpsEast, args[2], args[3], args[4], args[5])
}

func (o *OurChaincode) handleRatingCreation(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//args should be [EmailFrom, EmailTo, stars, message, orderid]
	if len(args) != 5 {
		s := "The method " + createRating + " expects 5 arguments. See documentation for help."
		log.Println(s)
		return nil, errors.New(s)
	}
	val, ok := strconv.Atoi(args[2])
	if ok != nil {
		s := "The parameter values for  " + createRating + " were incorrect. See documentation for help."
		log.Println(s)
		return nil, errors.New(s)
	}
	return nil, o.createRating(stub, strings.ToLower(args[0]), strings.ToLower(args[1]), val, args[3], args[4])
}

func (o *OurChaincode) handleSubscribtion(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//args: [carrier, offerid]
	if len(args) != 2 {
		s := "The method " + subscribe + " expects (at least) 2 parameters, see documentation for help"
		log.Println(s)
		return nil, errors.New(s)
	}
	orderbytes, err := stub.GetState(orders + "_" + args[1])
	if err != nil || len(orderbytes) == 0 {
		s := "Order not found"
		log.Println(s)
		return nil, errors.New(s)
	}
	var order order
	err = json.Unmarshal(orderbytes, &order)
	if err != nil {
		s := "Order could not be unmarshaled"
		log.Println(s)
		return nil, errors.New(s)
	}
	if order.IsBidOffer {
		_, err := strconv.Atoi(args[2])
		if err != nil {
			s := "The parameter values for method " + subscribe + " were wrong, see documentation for help"
			log.Println(s)
			return nil, errors.New(s)
		}
	}

	carrierbytes, err := stub.GetState(participants + "_" + strings.ToLower(args[0]))
	var carrier participant
	err = json.Unmarshal(carrierbytes, &carrier)
	if err != nil {
		s := "Carrier could not be unmarshaled"
		log.Println(s)
		return nil, errors.New(s)
	}

	return nil, o.subscribeCarrier(stub, carrier, order)
}

func (o *OurChaincode) handleCarrierSelect(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//args: [selectedCarrier, offerid]
	if len(args) != 2 {
		s := "The method " + selectCarrier + " expects (at least) 2 parameters, see documentation for help"
		log.Println(s)
		return nil, errors.New(s)
	}

	carBytes, err := stub.GetState(participants + "_" + strings.ToLower(args[0]))
	if err != nil || len(carBytes) == 0 {
		s := "Cannot find Carrier"
		log.Println(s)
		return nil, errors.New(s)
	}
	orderBytes, err := stub.GetState(orders + "_" + args[1])
	if err != nil || len(orderBytes) == 0 {
		s := "Cannot find Order"
		log.Println(s)
		return nil, errors.New(s)
	}

	var car participant
	var or order
	err = json.Unmarshal(carBytes, &car)
	if err != nil {
		s := "Error while Unmarshal Carrier"
		log.Println(s)
		return nil, errors.New(s)
	}
	err = json.Unmarshal(orderBytes, &or)
	if err != nil {
		s := "Error while Unmarshal Order"
		log.Println(s)
		return nil, errors.New(s)
	}

	return nil, o.selectCarrier(stub, car, or)
}

func (o *OurChaincode) handleOrderCompletion(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//order must be in status 'Assigned'
	//args: [orderId, FinishCode]
	if len(args) != 2 {
		s := "The Method " + finishOrder + " expects 2 Arguments, see documentation for help"
		log.Println(s)
		return nil, errors.New(s)
	}
	_, err := strconv.Atoi(args[0])
	if err != nil {
		s := "Parameter values in " + finishOrder + " were incorrect, see documentation for help"
		log.Println(s)
		return nil, errors.New(s)
	}
	orderBytes, err := stub.GetState(orders + "_" + args[0])
	if err != nil {
		s := "Order not found in " + finishOrder
		return []byte(s), errors.New(s)
	}
	var order order
	err = json.Unmarshal(orderBytes, &order)
	if err != nil {
		return nil, err
	}
	if strings.Compare(orderstatus[order.Status], "Started") != 0 {
		s := "Order in wrong status"
		log.Println(s)
		return nil, errors.New(s)
	}
	if strings.Compare(order.FinishCode, args[1]) != 0 {
		s := "The code was incorrect"
		log.Println(s)
		return nil, errors.New(s)
	}
	return nil, o.completeOrder(stub, order)
}

func (o *OurChaincode) giveCoins(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//args: [email, value]
	if len(args) != 2 {
		s := "Give coins expects the email and the value"
		log.Println(s)
		return nil, errors.New(s)
	}
	value, err := strconv.Atoi(args[1])
	if err != nil {
		s := "Error while converting 2nd value to integer in giveCoins"
		log.Println(s)
		return nil, err
	}
	pbytes, err := stub.GetState(participants + "_" + strings.ToLower(args[0]))
	if err != nil || len(pbytes) == 0 {
		s := "No participant found with email " + args[0]
		log.Println(s)
		if err != nil {
			return nil, err
		}
		return nil, errors.New(s)
	}
	//participant found check amount
	moneybytes, err := stub.GetState(money + participants + "_" + strings.ToLower(args[0]))
	if err != nil {
		s := "Error while getState in giveCoins"
		log.Println(s)
		return nil, err
	}
	moneys, err := strconv.Atoi(string(moneybytes[:]))
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	moneys += value
	moneystring := strconv.Itoa(moneys)
	err = stub.PutState(money+participants+"_"+strings.ToLower(args[0]), []byte(moneystring))
	if err != nil {
		log.Println("Error in PutState in giveCoins")
		return nil, err
	}

	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	if len(p.Transactions) == 0 || cap(p.Transactions)+1 > len(p.Transactions) {
		copy := make([]notification, len(p.Transactions), (cap(p.Transactions)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Transactions {
			copy[i] = p.Transactions[i]
		}
		p.Transactions = copy
	}
	p.Transactions = append(p.Transactions, notification{o.getIDForNotification(), time.Now(), "COINS_IN", []string{args[1]}})
	pbytes, err = json.Marshal(p)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	err = stub.PutState(participants+"_"+p.Email, pbytes)
	if err != nil {
		log.Println(err.Error())
	}
	return nil, err
}

func (o *OurChaincode) handleNotificationDelete(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 2 {
		s := "handleNotifcationDelete expects [notificationid, participant] as param"
		log.Println(s)
		return nil, errors.New(s)
	}
	intid, err := strconv.Atoi(args[0])
	if err != nil {
		s := "First value needs to be an integer"
		log.Println(s)
		return nil, errors.New(s)
	}
	pbytes, err := stub.GetState(participants + "_" + args[1])
	if err != nil || len(pbytes) == 0 {
		s := "Participant with email " + args[1] + " not found"
		log.Println(s)
		return nil, errors.New(s)
	}
	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		s := "Error unmarshalling participant"
		log.Println(s)
		return nil, errors.New(s)
	}
	id := big.NewInt(int64(intid))
	return o.deleteNotification(stub, id, p)
}

func (o *OurChaincode) deleteNotification(stub shim.ChaincodeStubInterface, id *big.Int, p participant) ([]byte, error) {
	index := -1
	for i := range p.Notifications {
		if p.Notifications[i].ID.Cmp(id) == 0 {
			index = i
			break
		}
	}
	if index == -1 {
		s := "Notification not found"
		log.Println(s)
		return nil, nil
	}
	copy := p.Notifications
	p.Notifications = p.Notifications[:index]
	for i := index + 1; i < len(p.Notifications); i++ {
		p.Notifications = append(p.Notifications, copy[i])
	}
	log.Println("Deleted Notification with id " + id.String())
	pval, err := o.participantToJSON(stub, &p)
	if err != nil || len(pval) == 0 {
		return nil, errors.New("participantstojson failed")
	}
	err = stub.PutState(participants+"_"+p.Email, pval)
	if err != nil {
		s := "putstate failed"
		log.Println(s)
		return nil, errors.New(s)
	}
	return nil, nil
}

func (o *OurChaincode) handleOrderStart(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	// args[orderid]
	if len(args) != 1 {
		s := "Invalid parameter length for OrderStart, expecting orderid"
		log.Println(s)
		return nil, errors.New(s)
	}
	orderBytes, err := stub.GetState(orders + "_" + args[0])
	if err != nil || len(orderBytes) == 0 {
		s := "No order found with id " + args[0]
		log.Println(s)
		return nil, errors.New(s)
	}
	var order order
	err = json.Unmarshal(orderBytes, &order)
	if err != nil {
		s := "Something went wrong unmarshalling the order"
		log.Println(s)
		return nil, errors.New(s)
	}
	if order.SelectedCarrier == "" {
		s := "No Carrier selected for order with id " + args[0]
		log.Println(s)
		return nil, errors.New(s)
	}
	order.Status = order.getNextStatus()
	order.ActualStart = time.Now()

	var p participant
	pbytes, err := stub.GetState(participants + "_" + order.Owner)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	//ORDER_STARTED
	if len(p.Notifications) == 0 || cap(p.Notifications)+1 > len(p.Notifications) {
		copy := make([]notification, len(p.Notifications), (cap(p.Notifications)+1)*2) //+1 in case cap(p.notifications) == 0
		for i := range p.Notifications {
			copy[i] = p.Notifications[i]
		}
		p.Notifications = copy
	}
	p.Notifications = append(p.Notifications, notification{o.getIDForNotification(), time.Now(), "ORDER_STARTED", []string{order.ID.String()}})
	pbytes, err = json.Marshal(p)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	err = stub.PutState(participants+"_"+order.Owner, pbytes)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}

	orderBytes, err = json.Marshal(order)
	if err != nil {
		s := "Something went wrong marshalling the order"
		log.Println(s)
		return nil, errors.New(s)
	}
	err = stub.PutState(orders+"_"+args[0], orderBytes)
	if err != nil {
		s := "Something went wrong putting state of  order"
		log.Println(s)
		return nil, errors.New(s)
	}
	return nil, nil
}

func (o *OurChaincode) getFinishOrderCode(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	if len(args) != 1 {
		s := "FinishOrderCode expects the order id as only param."
		log.Println(s)
		return nil, errors.New(s)
	}
	orderbytes, err := stub.GetState(orders + "_" + args[0])
	if len(orderbytes) == 0 || err != nil {
		s := "Order with id " + args[0] + " not found."
		log.Println(s)
		return nil, errors.New(s)
	}
	var or order
	err = json.Unmarshal(orderbytes, &or)
	if err != nil {
		log.Println(err.Error())
		return nil, errors.New(err.Error())
	}
	return []byte(or.FinishCode), nil
}

func (o *OurChaincode) participantByEmail(stub shim.ChaincodeStubInterface, email string) ([]byte, error) {
	email = strings.ToLower(email)
	pbytes, err := stub.GetState(participants + "_" + email)
	if len(pbytes) == 0 {
		err = errors.New("Cannot find Participant with email " + email)
	}
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	var p participant
	err = json.Unmarshal(pbytes, &p)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	return pbytes, nil
}

func (o *OurChaincode) getCoins(stub shim.ChaincodeStubInterface, email string) ([]byte, error) {
	email = strings.ToLower(email)
	bytes, err := stub.GetState(money + participants + "_" + email)
	if err != nil {
		log.Println(err.Error())
		return nil, err
	}
	return bytes, nil
}

func (o *OurChaincode) getRandomString(n int) string {
	var letters = []rune("abcdefghijklmnopqrstuvwxyz1234567890")
	var ret string
	for i := 0; i < n; i++ {
		s := string(letters[rand.Intn(len(letters))])
		random := rand.Intn(2)
		if random > 0 {
			s = strings.ToUpper(s)
		}
		ret += s
	}
	return ret
}

//**************************************************//
//     Blockchain / Smart Contract Methods          //
//**************************************************//

//Init this Smart Contract
func (o *OurChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	return nil, nil
}

//Invoke is called with the function Name as argument. Invoke calls the given method with params args
func (o *OurChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if strings.EqualFold(function, createOrder) {
		return o.handleOrderCreation(stub, args)
	}
	if strings.EqualFold(function, createParticipant) {
		return o.handleParticipantCreation(stub, args)
	}
	if strings.EqualFold(function, subscribe) {
		return o.handleSubscribtion(stub, args)
	}
	if strings.EqualFold(function, createRating) {
		return o.handleRatingCreation(stub, args)
	}
	if strings.EqualFold(function, selectCarrier) {
		return o.handleCarrierSelect(stub, args)
	}
	if strings.EqualFold(function, finishOrder) {
		return o.handleOrderCompletion(stub, args)
	}
	if strings.EqualFold(function, coins) {
		return o.giveCoins(stub, args)
	}
	if strings.EqualFold(function, "deletenotification") {
		return o.handleNotificationDelete(stub, args)
	}
	if strings.EqualFold(function, "order_start") {
		return o.handleOrderStart(stub, args)
	}
	if strings.EqualFold(function, "order_delete") {
		return o.deleteOrder(stub, args)
	}
	log.Println("Invoke did not find function: " + function)
	return nil, errors.New("Invoke did not find function")
}

//Query values
func (o *OurChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if strings.EqualFold(function, "allOrders") {
		return o.ordersToJSON(stub, nil, "Created", []int64{}, false)
	} else if strings.EqualFold(function, "mycreatedOrders") {
		return o.ordersByParticipant(stub, args[0])
	} else if strings.EqualFold(function, "myassignedOrders") {
		return o.ordersByCarrier(stub, args[0], []int{1, 2})
	} else if strings.EqualFold(function, "mystartedOrders") {
		return o.ordersByCarrier(stub, args[0], []int{2})
	} else if strings.EqualFold(function, "orderById") {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			s := "Error converting " + args[0] + " to int"
			log.Println(s)
			return nil, errors.New(s)
		}
		return o.ordersToJSON(stub, nil, "", []int64{int64(id)}, false)
	} else if strings.EqualFold(function, "mynotifications") {
		return o.notificationsByParticipant(stub, args[0])
	} else if strings.EqualFold(function, "mytransactions") {
		return o.transactionsByParticipant(stub, args[0])
	} else if strings.EqualFold(function, "isregistered") {
		if len(args) != 2 {
			s := "Expect Email and password of Participant"
			log.Println(s)
			return nil, errors.New(s)
		}
		return o.isRegistered(stub, args[0], args[1])
	} else if strings.EqualFold(function, "getOrderCode") {
		return o.getFinishOrderCode(stub, args)
	} else if strings.EqualFold(function, "participantbyemail") {
		return o.participantByEmail(stub, args[0])
	} else if strings.EqualFold(function, "getCoins") {
		return o.getCoins(stub, args[0])
	} else if function != "" { //"" is empty / default value -> no error here
		log.Println("Hello there " + function)
		return []byte("42"), nil
	}
	log.Println("query did not find func: " + function)
	return nil, errors.New("Received unknown function query: " + function)
}

//Main Function here
func main() {
	err := shim.Start(new(OurChaincode))
	if err != nil {
		log.Println("Error starting the Chaincode:" + err.Error())
	}
}
