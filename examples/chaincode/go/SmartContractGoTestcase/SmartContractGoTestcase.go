package main

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric/core/chaincode/shim"
)

//**************************************************//
//              Global variables                    //
//**************************************************//

var status = [...]string{
	"InCreation",
	"Created",
	"Assignable",
	"Assigned",
	"Finished",
}

//**************************************************//
//            Method Names in INIT                  //
//**************************************************//

const initName = "INIT"
const query = "QUERY"
const createOrder = "CREATE_ORDER"
const finishOrderCreation = "FINISH_ORDER_CREATION"
const createParticipant = "CREATE_PARTICIPANT"
const createRating = "CREATE_RATING"
const subscribe = "CARRIER_SUBSCRIBE"
const selectCarrier = "SELECT_CARRIER"
const finishOrder = "COMPLETE_ORDER"

//**************************************************//
//                  Structs                         //
//**************************************************//

//OurChaincode is our main struct that has (and manages) everything
type OurChaincode struct {
	allParticipants map[string]participant
	orderCount      int
	ordersByID      map[int64][]order
}

type rating struct {
	from  participant
	stars int
	text  string
}

type geoLocation struct {
	gpsNorth float64
	gpsEast  float64
}

type adress struct {
	geolocation *geoLocation
}

type participant struct {
	adress      *adress
	name        string
	ratings     []rating
	averageRate float64
	email       string //key in map
	orders      []int64
}

type order struct {
	id                     *big.Int
	owner                  participant
	carriers               []participant
	isBidOffer             bool
	status                 int
	endOfSubscribtionOrBid time.Time
	selectedCarrier        participant
	price                  int //in cent
}

//**************************************************//
//             Help Methods INIT                    //
//**************************************************//

//NewParticipant creates a new Participant if no Participant exists with the given email
func (o *OurChaincode) NewParticipant(gpsNorth float64, gpsEast float64, name string, email string) error {
	return o.CreateParticipant(gpsNorth, gpsEast, name, email)
}

//CreateParticipant creates a Participant with a location, a name and an email adress, if not exists.
func (o *OurChaincode) CreateParticipant(gpsNorth float64, gpsEast float64, name string, email string) error {
	_, ok := o.allParticipants[email]
	if ok {
		// do not add a participant twice
		return nil
	}
	if gpsNorth != 0 && gpsEast != 0 && name != "" && email != "" {
		p := participant{}
		p.name = name
		p.ratings = make([]rating, 0, 10)
		p.email = email
		//set location to gps values
		loc := new(geoLocation)
		loc.gpsNorth = gpsNorth
		loc.gpsEast = gpsEast
		//add location to adress and adress to participant
		a := new(adress)
		a.geolocation = loc
		p.adress = a

		//add to map
		o.allParticipants[email] = p
		return nil
	}
	return errors.New("Invalid argument values")

}

//NewOrder creates an order with the given owner.
//Returns order reference and error if failure.
func (o *OurChaincode) NewOrder(email string, bid bool, price int) (order, error) {
	p, ok := o.allParticipants[email]
	if ok {
		or := order{}
		or.owner = p
		or.carriers = make([]participant, 0, 10)
		or.id = o.getIDForOrder()
		or.isBidOffer = bid
		or.status = 0
		or.selectedCarrier = participant{}
		or.price = price

		//add to all orders
		if o.ordersByID[or.id.Int64()] == nil || len(o.ordersByID[or.id.Int64()]) == 0 || cap(o.ordersByID[or.id.Int64()])+1 > len(o.ordersByID[or.id.Int64()]) {
			copy := make([]order, len(o.ordersByID[or.id.Int64()]), (cap(o.ordersByID[or.id.Int64()])+1)*2)
			for i := range o.ordersByID[or.id.Int64()] {
				copy[i] = o.ordersByID[or.id.Int64()][i]
			}
			o.ordersByID[or.id.Int64()] = copy
		}
		o.ordersByID[or.id.Int64()] = append(o.ordersByID[or.id.Int64()], or)
		o.orderCount++

		//add to participant list of orderids
		if p.orders == nil || len(p.orders) == 0 || cap(p.orders)+1 > len(p.orders) {
			copy := make([]int64, len(p.orders), (cap(p.orders)+1)*2) //+1 in case caps(p.ratings) == 0
			for i := range p.orders {
				copy[i] = p.orders[i]
			}
			p.orders = copy
		}
		p.orders = append(p.orders, or.id.Int64())
		return or, nil
	}
	return order{}, errors.New("Invalid argument: expecting email of owner")
}

//createRating adds an rating with the given stars and the given text to the given participant.
func (o *OurChaincode) createRating(from string, to string, stars int, text string) error {
	pFrom, ok := o.allParticipants[from]
	if !ok {
		//should not be happening, but maybe......
		fmt.Println("nil check failed in createRating, from adress is wrong")
		return errors.New("nil check failed in createRating from adress is wrong")
	}

	pTo, ok := o.allParticipants[to]
	if !ok || pTo.ratings == nil {
		//should not be happening, but maybe......
		fmt.Println("nil check failed in createRating, to adress is wrong")
		return errors.New("nil check failed in createRating, to adress is wrong")
	}

	//for remembering: len -> used space in the slice
	// cap -> length of the slice [1,0,0] -> len = 1, cap = 3
	if len(pTo.ratings) == 0 || len(pTo.ratings)+1 > cap(pTo.ratings) {
		//make slice / array bigger
		copy := make([]rating, len(pTo.ratings), (cap(pTo.ratings)+1)*2) //+1 in case caps(p.ratings) == 0
		for i := range pTo.ratings {
			copy[i] = pTo.ratings[i]
		}
		pTo.ratings = copy
	}
	r := rating{pFrom, stars, text}
	pTo.ratings = append(pTo.ratings, r)
	pTo.averageRate = o.getAverage(pTo)
	return nil
}

func (o *OurChaincode) subscribeCarrier(carrier string, orderID int) error {
	car, ok := o.allParticipants[carrier]
	if !ok {
		s := "Carrier not found in " + subscribe
		fmt.Println(s)
		return errors.New(s)
	}

	order := o.orderByID(big.NewInt(int64(orderID)))
	if order.id.Cmp(big.NewInt(0)) < 1 {
		//if id is < 0 the result will be -1
		//and if id is = 0 the result will be 0
		//in both cases we have no order found
		s := "Order not found in " + subscribe
		fmt.Println(s)
		return errors.New(s)
	}
	found := false
	for j := 0; j < len(order.carriers); j++ {
		if order.carriers[j].equals(car) {
			found = true
			break
		}
	}
	if !found {
		order.carriers = append(order.carriers, o.allParticipants[carrier])
	}
	return nil
}

func (o *OurChaincode) selectCarrier(carrier string, orderID int) error {
	car, ok := o.allParticipants[carrier]
	if !ok {
		s := "Carrier not found in " + selectCarrier
		fmt.Println(s)
		return errors.New(s)
	}

	order := o.orderByID(big.NewInt(int64(orderID)))
	if order.id.Cmp(big.NewInt(0)) < 1 {
		//if id is < 0 the result will be -1
		//and if id is = 0 the result will be 0
		//in both cases we have no order found
		s := "Order not found in " + selectCarrier
		fmt.Println(s)
		return errors.New(s)
	}
	order.selectedCarrier = car
	order.carriers = make([]participant, 0, 0)

	//todo message to carrier
	return nil
}

func (p *participant) equals(p2 participant) bool {
	if p == nil || p.name == "" || p2.name == "" {
		return false
	}
	return strings.Compare(strings.ToUpper(p2.name), strings.ToUpper(p.name)) == 0
}

//CompleteOrder complets the order
func (o *OurChaincode) CompleteOrder(stub shim.ChaincodeStubInterface, order order) error {
	order.status = order.getNextStatus()
	owner, err := stub.GetState(order.owner.email)
	if err != nil {
		s := "GetState for owner failed in method " + finishOrder
		fmt.Println(s)
		return errors.New(s)
	}
	ownerMoney := convert(owner)
	carrier, err := stub.GetState(order.carriers[0].email)
	if err != nil {
		s := "GetState for carrier failed in method " + finishOrder
		fmt.Println(s)
		return errors.New(s)
	}
	carrierMoney := convert(carrier)
	ownerMoney = ownerMoney - order.price
	carrierMoney = carrierMoney + order.price
	err = stub.PutState(order.owner.email, []byte{uint8(ownerMoney)})
	err1 := stub.PutState(order.carriers[0].email, []byte{uint8(carrierMoney)})

	if err != nil || err1 != nil {
		s := "something went wrong saving the balance"
		fmt.Println(s)
		return errors.New(s)
	}
	return nil
}

//convert converts a byte slices first value to a int
func convert(b []byte) int {
	return int(b[0])
}

//**************************************************//
//              Query  Functions                    //
//**************************************************//

//getNextStatus returns the next Status for an order
func (o *order) getNextStatus() int {
	return o.status + 1
}

func (o *OurChaincode) orderByID(orderID *big.Int) order {
	list := o.ordersByID[orderID.Int64()]
	for i := 0; i < len(list); i++ {
		if list[i].id.Cmp(orderID) == 0 {
			return list[i]
		}
	}
	return order{}
}

//getIDForOrder returns the id for another order
func (o *OurChaincode) getIDForOrder() *big.Int {
	if o.orderCount == 0 {
		return big.NewInt(1)
	}
	return big.NewInt(int64(o.orderCount) + 1)
}

//getAverage returns the average rating as float64 of the given participant
func (o *OurChaincode) getAverage(p participant) float64 {
	var average int //default 0
	if p.name != "" && len(p.ratings) > 0 {
		for i := 0; i < len(p.ratings); i++ {
			average = average + p.ratings[i].stars
		}
		return float64(average) / float64(len(p.ratings))
	}
	return 0
}

//**************************************************//
//      Methods called from Invoke / API            //
//**************************************************//

//HandleOrderCreation handles the api call for creation of orders
func (o *OurChaincode) HandleOrderCreation(args []string) ([]byte, error) {
	//args: "emailofshipper, isbidoffer, price"
	if len(args) != 3 {
		s := "Invalid argument count for " + createOrder + " , see documentation for help"
		return nil, errors.New(s)
	}
	bid, err := strconv.ParseBool(args[1])
	if err != nil {
		fmt.Println("2nd value must be boolean in method " + createOrder)
		return nil, errors.New("2nd value must be boolean in method " + createOrder)
	}
	val, err := strconv.Atoi(args[2])
	if err != nil {
		s := "3rd value must be integer value, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}

	_, err = o.NewOrder(args[0], bid, val)
	if err != nil {
		fmt.Println("Error in " + createOrder + ": No user found for email: " + args[0])
		return nil, err
	}
	//TODO maybe show order values or something
	return nil, nil
}

//HandleFinishOrderCreation handles the Finish of Order Creation
func (o *OurChaincode) HandleFinishOrderCreation(args []string) ([]byte, error) {
	//args: [shipperEmail, orderID, EndTimeOfSubscribeOrBid yyyy-mm-dd-hh--mm ]
	if len(args) != 3 {
		s := "The Method " + finishOrderCreation + " expects 2 arguments, see documentation for help."
		fmt.Println(s)
		return nil, errors.New(s)
	}
	val, err := strconv.Atoi(args[1])
	d := strings.Split(args[2], "-")
	if err != nil || len(d) != 5 {
		s := "Wrong parameter values in method " + finishOrderCreation + " see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	order := o.orderByID(big.NewInt(int64(val)))
	if order.id.Cmp(big.NewInt(0)) < 1 {
		s := "Order with id " + args[1] + " not found in method " + finishOrderCreation
		fmt.Println(s)
		return nil, errors.New(s)
	}
	if order.status >= 1 {
		s := "Order with id " + args[1] + " has wrong status " + status[order.status] + " in Method " + finishOrderCreation
		fmt.Println(s)
		return nil, errors.New(s)
	}
	order.status = order.getNextStatus()
	t, err := time.Parse("2016-Jan-02 12:34:02", args[2])
	if err != nil {
		s := "Something went wrong parsing date in " + finishOrderCreation
		fmt.Println(s)
		return nil, errors.New(s)
	}
	if !t.After(time.Now()) {
		s := "Invalid date in " + finishOrderCreation
		fmt.Println(s)
		return nil, errors.New(s)
	}
	order.endOfSubscribtionOrBid = t
	return nil, nil
}

//HandleParticipantCreation handles participant creation
func (o *OurChaincode) HandleParticipantCreation(args []string) ([]byte, error) {
	if len(args) != 4 {
		s := "Function HandleParticipantCreation expects 4 arguments, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	gpsNorth, err := strconv.ParseFloat(args[0], 64)
	gpsEast, err2 := strconv.ParseFloat(args[1], 64)
	if err != nil || err2 != nil {
		s := "The first an second value must be a float64 value"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	err = o.NewParticipant(gpsNorth, gpsEast, args[2], args[3])
	if err != nil {
		//log error here before return
		fmt.Println(err)
	}
	return nil, err
}

//HandleRatingCreation handles the api call for rating creation
func (o *OurChaincode) HandleRatingCreation(args []string) ([]byte, error) {
	//args should be [emailFrom, emailTo, stars, message]
	if len(args) != 4 {
		s := "The method " + createRating + " expects 4 arguments. See documentation for help."
		fmt.Println(s)
		return nil, errors.New(s)
	}
	val, ok := strconv.Atoi(args[2])
	if ok != nil {
		s := "The parameter values for  " + createRating + " were incorrect. See documentation for help."
		fmt.Println(s)
		return nil, errors.New(s)
	}
	err := o.createRating(args[0], args[1], val, args[3])
	return nil, err
}

//HandleSubscribtion handles the participant who wants to do an offer
func (o *OurChaincode) HandleSubscribtion(args []string) ([]byte, error) {
	//args: [carrier, offerid] or [carrier, offerid, price] (not yet implemented)
	if len(args) < 2 || len(args) > 3 {
		s := "The method " + subscribe + " expects (at least) 2 parameters, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	offer, err := strconv.Atoi(args[2])
	if err != nil {
		s := "The parameter values for method " + subscribe + " were wrong, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	return nil, o.subscribeCarrier(args[0], offer)
}

//HandleCarrierSelect handles the participant who wants to do an offer
func (o *OurChaincode) HandleCarrierSelect(args []string) ([]byte, error) {
	//args: [selectedCarrier, offerid]
	if len(args) != 2 {
		s := "The method " + selectCarrier + " expects (at least) 3 parameters, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	offer, err := strconv.Atoi(args[1])
	if err != nil {
		s := "The parameter values for method " + selectCarrier + " were wrong, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	return nil, o.selectCarrier(args[0], offer)
}

//HandleOrderCompletion handles the order completion
func (o *OurChaincode) HandleOrderCompletion(stub shim.ChaincodeStubInterface, args []string) ([]byte, error) {
	//order must be in status 'Assigned'
	//args: [orderId]
	if len(args) != 1 {
		s := "The Method " + finishOrder + " expects 1 Argument, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	val, err := strconv.Atoi(args[0])
	if err != nil {
		s := "Parameter values in " + finishOrder + " were incorrect, see documentation for help"
		fmt.Println(s)
		return nil, errors.New(s)
	}
	order := o.orderByID(big.NewInt(int64(val)))
	if strings.Compare(status[order.status], "Assigned") != 0 {
		s := "Order in wrong status"
		fmt.Println(s)
		return nil, errors.New(s)
	}

	return nil, o.CompleteOrder(stub, order)
}

//**************************************************//
//     Blockchain / Smart Contract Methods          //
//**************************************************//

//Init this Smart Contract
func (o *OurChaincode) Init(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	if o.orderCount == 0 {
		o.ordersByID = make(map[int64][]order)
		o.allParticipants = make(map[string]participant)
	}
	return nil, nil
}

//Invoke is called with the function name as argument. Invoke calls the given method with params args
func (o *OurChaincode) Invoke(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	//fmt.Println("Invoke is running " + function)
	if function == initName {
		return o.Init(stub, initName, args)
	}
	if function == query {
		return o.Query(stub, query, args)
	}
	if function == createOrder {
		return o.HandleOrderCreation(args)
	}
	if function == finishOrderCreation {
		return o.HandleFinishOrderCreation(args)
	}
	if function == createParticipant {
		return o.HandleParticipantCreation(args)
	}
	if function == subscribe {
		return o.HandleSubscribtion(args)
	}
	if function == createRating {
		return o.HandleRatingCreation(args)
	}
	if function == selectCarrier {
		return o.HandleCarrierSelect(args)
	}
	if function == finishOrder {
		return o.HandleOrderCompletion(stub, args)
	}

	fmt.Println("Invoke did not find function: " + function)
	return nil, nil //errors.New("Received unknown function invocation: " + function)
}

//Query values
func (o *OurChaincode) Query(stub shim.ChaincodeStubInterface, function string, args []string) ([]byte, error) {
	fmt.Println("query is running: " + function)

	if function != "" { //"" is empty / default value -> no error here
		fmt.Println("Hello there " + function)
		return nil, nil
	}
	fmt.Println("query did not find func: " + function)
	return nil, errors.New("Received unknown function query: " + function)
}

//Main Function here
func main() {
	err := shim.Start(new(OurChaincode))
	if err != nil {
		fmt.Printf("Error starting the Chaincode: %s", err)
	}
}
