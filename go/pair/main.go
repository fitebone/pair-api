package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"math/rand"
	"net/mail"
	"time"

	//"io/ioutil"
	"log"
	"net"
	"os"
	"strings"

	b64 "encoding/base64"
	pb "pair/pairapi"

	"github.com/joho/godotenv"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"gopkg.in/auth0.v5/management"
)

var (
	port        = flag.Int("port", 50051, "The server port")
	iss         string
	aud         string
	auth0PEM    []byte
	auth0m      *management.Management
	auth0domain string
	auth0id     string
	auth0code   string
	client      *mongo.Client // Client pool
	contx, _    = context.WithTimeout(context.Background(), 10*time.Second)
	ctx_k       = ctx_key("user_id")
	letters     = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_=+()[]{}*&^%$#@!~`\\|/?.>,<")
)

type ctx_key string

type Account struct {
	Id       string //`bson:"first_name,omitempty"`
	Email    string
	Username string
	Created  int64
	Pic      string
	Points   int32
	Peers    int32
}

type PairStart struct {
	Id      string
	PeerId  string
	Secret  string
	Created time.Time
}

type Pair struct {
	Peer1    string // confirmer
	Peer2    string
	Created  time.Time
	Verified bool
}

type server struct {
	pb.UnimplementedPairAPIServer
}

/////////////////////////////
// PAIR API gRPC FUNCTIONS //
/////////////////////////////

// ACCOUNT //
func (s *server) CreateAccount(ctx context.Context, in *pb.AccountCreateReq) (*pb.AccountCreateResp, error) {
	logPeer(ctx, "CreateAccount")
	if !verifyID(ctx, in.Id) {
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	// Check if account exists
	coll := client.Database("pair_db").Collection("accounts")
	found := coll.FindOne(ctx, bson.D{{"id", in.Id}})
	if found.Err() != mongo.ErrNoDocuments {
		return nil, status.Errorf(codes.AlreadyExists, fmt.Sprintf("Account %s exists", in.Id))
	}

	// TODO: Check argument validity (like email)
	// If not, create account
	doc := Account{
		Id:       in.Id,
		Email:    in.Email,
		Username: "Anonymous",
		Created:  time.Now().Unix(),
		Pic:      in.Pic,
		Points:   0,
		Peers:    0,
	}

	result, err := coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Insert fail")
	}
	fmt.Printf("Account doc inserted with ID: %s", result.InsertedID)

	return &pb.AccountCreateResp{
		Id:       doc.Id,
		Email:    doc.Email,
		Username: doc.Username,
		Created:  doc.Created,
		Pic:      doc.Pic,
		Points:   doc.Points,
		Peers:    doc.Peers,
	}, nil
}

func (s *server) GetAccount(ctx context.Context, in *pb.AccountGetReq) (*pb.AccountGetResp, error) {
	logPeer(ctx, "GetAccount")
	if !verifyID(ctx, in.Id) {
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	coll := client.Database("pair_db").Collection("accounts")
	var result Account
	err := coll.FindOne(ctx, bson.D{{"id", in.Id}}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			// TODO: Make responses more ambiguous (Internal etc)
			return nil, status.Errorf(codes.NotFound, fmt.Sprintf("Account %s not found", in.Id))
		}
		return nil, status.Errorf(codes.Internal, fmt.Sprintf("Find failed %s", in.Id))
	}

	return &pb.AccountGetResp{
		Id:       result.Id,
		Email:    result.Email,
		Username: result.Username,
		Created:  result.Created,
		Pic:      result.Pic,
		Points:   result.Points,
		Peers:    result.Peers,
	}, nil
}

func (s *server) UpdateAccount(ctx context.Context, in *pb.AccountUpdateReq) (*pb.AccountUpdateResp, error) {
	logPeer(ctx, "UpdateAccount")
	if !verifyID(ctx, in.Id) {
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	// Check if account exists
	coll := client.Database("pair_db").Collection("accounts")
	found := coll.FindOne(ctx, bson.D{{"id", in.Id}})
	if found.Err() == mongo.ErrNoDocuments {
		return nil, status.Errorf(codes.NotFound, "Account does not exist")
	}

	filter := bson.D{{"id", in.Id}}
	switch in.Column {
	case "email":
		if !validEmail(in.Data) {
			return nil, status.Errorf(codes.InvalidArgument, "Bad email formatting")
		}
	case "username":
		update := bson.D{{"$set", bson.D{{"username", in.Data}}}}
		coll.UpdateOne(ctx, filter, update)
	case "pic":
		// TODO: Check validity of image
		//update := bson.D{{"$set", bson.D{{"pic", in.Data}}}}
		//coll.UpdateOne(ctx, filter, update)
	default:
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	update := bson.D{{"$set", bson.D{{in.Column, in.Data}}}}
	result, err := coll.UpdateOne(ctx, filter, update)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Update Account fail")
	}
	fmt.Printf("Account doc updated with ID: %s", result.UpsertedID)

	return &pb.AccountUpdateResp{
		Id:     in.Id,
		Column: in.Column,
		Data:   in.Data,
	}, nil
}

// END ACCOUNT //

// PAIR //
func (s *server) StartPair(ctx context.Context, in *pb.PairStartReq) (*pb.PairStartResp, error) {
	logPeer(ctx, "StartPair")
	if !verifyID(ctx, in.Id) {
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	// Check that peer_id != id
	if in.Id == in.PeerId {
		return nil, status.Errorf(codes.InvalidArgument, "Pairing with self")
	}

	// Check if pair start exists
	coll := client.Database("pair_db").Collection("pairs_temp")
	found := coll.FindOne(ctx, bson.D{{"id", in.Id}, {"peerid", in.PeerId}})
	if found.Err() != mongo.ErrNoDocuments {
		return nil, status.Errorf(codes.AlreadyExists, "Pair start exists")
	}

	// TODO: Check if peer_id is real user
	//_, err := auth0m.Client.Read(in.Id)
	//if err != nil {
	//	return nil, status.Errorf(codes.NotFound, "Account doesnt exist")
	//}

	secret := generateSecret()
	doc := PairStart{
		Id:      in.Id,
		PeerId:  in.PeerId,
		Secret:  secret,
		Created: time.Now(),
	}
	result, err := coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Insert StartPair fail")
	}
	fmt.Printf("PairStart doc inserted with ID: %s", result.InsertedID)

	return &pb.PairStartResp{
		PeerId: in.PeerId,
		Secret: secret,
	}, nil
}

func (s *server) FinishPair(ctx context.Context, in *pb.PairFinishReq) (*pb.PairFinishResp, error) {
	logPeer(ctx, "FinishPair")
	if !verifyID(ctx, in.Id) {
		return nil, status.Errorf(codes.PermissionDenied, "Request denied")
	}

	// Check that other user's pair_temp doc exists
	coll := client.Database("pair_db").Collection("pairs_temp")
	found := coll.FindOne(ctx, bson.D{{"id", in.PeerId}, {"peerid", in.Id}})
	if found.Err() == mongo.ErrNoDocuments {
		return nil, status.Errorf(codes.NotFound, "PairStart doesnt exist")
	}

	var result PairStart
	// Swap the id and peerid since secret is shared through Nearby
	filter := bson.D{{"id", in.PeerId}, {"peerid", in.Id}, {"secret", in.Secret}}
	err := coll.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "PairStart not found") //fmt.Sprintf
		}
		return nil, status.Errorf(codes.Internal, "Find PairStart failed %s")
	}

	// Confirm new pair
	coll = client.Database("pair_db").Collection("pairs")
	now := time.Now()
	doc := Pair{
		Peer1:    in.Id,
		Peer2:    in.PeerId,
		Created:  now,
		Verified: true,
	}
	_, err = coll.InsertOne(ctx, doc)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "Insert Pair fail")
	}

	return &pb.PairFinishResp{
		Created: now.Unix(),
	}, nil
}

// END PAIR //

/////////////////////////////
//    END PAIR API gRPC    //
/////////////////////////////

func main() {
	//log.SetOutput(ioutil.Discard)
	//flags.Parse()

	// ENV SETUP //
	err := godotenv.Load()
	if err != nil {
		panic(err) //panic("Error loading .env file")
	}
	iss = os.Getenv("iss")
	aud = os.Getenv("aud")
	auth0PEM, _ = b64.StdEncoding.DecodeString(os.Getenv("auth0pem"))
	auth0domain = os.Getenv("auth0domain")
	auth0id = os.Getenv("auth0id")
	auth0code = os.Getenv("auth0code")

	// MONGO SETUP //
	uri := os.Getenv("mungo")
	client, err = mongo.Connect(contx, options.Client().ApplyURI(uri))
	if err != nil {
		panic(err) //panic("Error spinning up client | mongoDB")
	}
	if err = client.Ping(contx, readpref.Primary()); err != nil {
		panic(err) //panic("Error pinging | mongoDB")
	}
	defer client.Disconnect(contx)
	//temp_pairs := client.Database("pair_db").Collection("pairs_temp")
	//index := mongo.IndexModel{
	//	Keys: bsonx.Doc{{Key: "created", Value: bsonx.Int32(1)}},
	//	// Expire document after 30 seconds?
	//	Options: options.Index().SetExpireAfterSeconds(30),
	//}
	//_, err = temp_pairs.Indexes().CreateOne(context.Background(), index)
	//if err != nil {
	//	panic(err) //Error setting up TTL | mongoDB
	//}

	// AUTH0 SETUP //
	auth0m, err = management.New(auth0domain, management.WithClientCredentials(auth0id, auth0code))
	if err != nil {
		panic(err) //panic("Failed to listen | net")
	}

	// SERVER SETUP //
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		panic("Failed to listen | net")
	}

	// TLS SETUP //
	tlsCredentials, err := loadTLSCredentials()
	if err != nil {
		panic(err) //panic("Could not load credentials | tls")
	}

	// gRPC SETUP //
	s := grpc.NewServer(
		grpc.Creds(tlsCredentials),
		grpc.UnaryInterceptor(AuthInterceptor),
	)
	pb.RegisterPairAPIServer(s, &server{})
	log.Printf("Server listening at %s | Pair API", lis.Addr().String())
	if err := s.Serve(lis); err != nil {
		panic("Failed to serve | gRPC")
	}

}

////////////////////////////
//   UTILITY FUNCTIONS    //
////////////////////////////

// Generate secret for Pairing process
func generateSecret() string {
	s := make([]rune, 16)
	for i := range s {
		s[i] = letters[rand.Intn(len(letters))]
	}
	return string(s)
}

// Validate the formatting of email address
func validEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// Verify the auth token id is the same as the id argument in the gRPC message
func verifyID(ctx context.Context, id string) bool {
	req_id := ctx.Value(ctx_k)
	return req_id == id
}

// TODO: Figure out what "info" (UnaryServerInfo) is for
func AuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Get metadata
	md, _ := metadata.FromIncomingContext(ctx)
	if md.Len() <= 0 {
		return nil, status.Errorf(codes.Unauthenticated, "Request unauthenticated")
	}
	// Get token from authorization header
	token := md["authorization"]
	// Decode PEM into bytes and verify auth token
	decoded, _, _ := jwk.DecodePEM(auth0PEM)
	verifiedToken, verify_err := jwt.ParseString(strings.Join(token, ""), jwt.WithKey(jwa.RS256, decoded))
	if verify_err != nil {
		_ = verifiedToken
		fmt.Printf("Failed to verify JWS: %s\n", verify_err)
		return nil, status.Errorf(codes.Unauthenticated, "Request unauthenticated")
	}
	// Validate the verified token
	jwt_err := jwt.Validate(verifiedToken, jwt.WithIssuer(iss), jwt.WithAudience(aud))
	if jwt_err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "Request unauthenticated")
	}
	return handler(context.WithValue(ctx, ctx_k, verifiedToken.Subject()), req)
}

func logPeer(ctx context.Context, func_name string) {
	p, p_err := peer.FromContext(ctx)
	if p_err {
		log.Printf("%s from user %s at %v", func_name, ctx.Value(ctx_k), p.Addr)
	}
}

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	// Load server's certificate and private key
	//serverCert, err := tls.LoadX509KeyPair(cert_path, key_path)
	serverCert, err := tls.LoadX509KeyPair("cert/cert.pem", "cert/pkey.pem")
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.NoClientCert, // Only serverside TLS
	}

	return credentials.NewTLS(config), nil
}

/*
func newPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:   80,
		MaxActive: 12000,
		Dial: func() (redis.Conn, error) {
			// Sign certification with private key
			clientCert, err := tls.X509KeyPair(cert, privateKey)
			if err != nil {
				panic(err)
			}
			// Create certification authority pool
			rootCAPool := x509.NewCertPool()
			ok := rootCAPool.AppendCertsFromPEM(rootCA)
			if !ok {
				panic("unable to append supplied cert into tls.Config, are you sure it is a valid certificate")
			}
			// Create the redis DB connection utilizing TLS
			c, err := redis.DialURL(
				os.Getenv("redis"),
				redis.DialTLSConfig(&tls.Config{
					Certificates: []tls.Certificate{clientCert},
					RootCAs:      rootCAPool,
				}),
			)
			if err != nil {
				panic(err.Error())
			}
			return c, err
		},
	}
}*/

/*
func mustOpenAndReadFile(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("unable to open file %s: %s", path, err))
	}
	b, err := ioutil.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("unable to ReadAll of file %s: %s", path, err))
	}
	return b
}*/
