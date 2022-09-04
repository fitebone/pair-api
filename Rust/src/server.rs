use redis::{Connection, Commands, Client};
use tonic::{transport::Server, Request, Response, Status};
use jsonwebtoken::{decode, Algorithm, DecodingKey, Validation, errors::ErrorKind};
use dotenv;
use std::env;
use serde::{Deserialize, Serialize};
use std::collections::hash_map::HashMap;
//use async_once::AsyncOnce;

use pairapi_rpc::pair_api_server::{PairApi, PairApiServer};
use pairapi_rpc::{
    AccountCreateReq, AccountCreateResp, 
    AccountGetReq, AccountGetResp
};

#[macro_use]
extern crate lazy_static;

lazy_static! {
    static ref CLIENT: Client = Client::open(env::var("redis").unwrap()).unwrap();
    static ref REDIS: Connection = CLIENT.get_connection().unwrap();
    /*static ref REDIS: AsyncOnce<MultiplexedConnection> = AsyncOnce::new(async{
        CLIENT.get_multiplexed_tokio_connection().await.unwrap()
    });*/
}

pub mod pairapi_rpc {
    tonic::include_proto!("pairapi");
}

#[derive(Default)]
pub struct MyPairApi {}

#[tonic::async_trait]
impl PairApi for MyPairApi {
    // TODO: MAKE GLOBAL ERROR HANDLER???
    // Create Account //
    async fn create_account(&self, req: Request<AccountCreateReq>,) -> Result<Response<AccountCreateResp>, Status> {
        println!("Create Account from {:?}", req.remote_addr());
        let message = req.into_inner();
        let key = message.id;
        let res = AccountCreateResp{};

        match CLIENT.get_connection().unwrap().hset_multiple::<String, String, String, String>(
            key, 
            &[
                ("username".to_string(), message.username),
                ("created".to_string(), message.created),
                ("points".to_string(), message.points)
            ]) {
                Err(_) => Err(Status::internal("Serious error")),
                Ok(_) => Ok(Response::new(res))
            }
    }
    // Get Account //
    async fn get_account(&self, req: Request<AccountGetReq>,) -> Result<Response<AccountGetResp>, Status> {
        println!("Get Account from {:?}", req.remote_addr());
        let message = req.into_inner();
        let key = message.id;
        match CLIENT.get_connection().unwrap().hgetall::<String, HashMap<String,String>>(key) {
            Err(_) => Err(Status::not_found("Resource not found")),
            Ok(result) => {
                let res = AccountGetResp {
                    email: result.get("email").unwrap().to_string(),
                    name: result.get("name").unwrap().to_string(),
                    created: result.get("created").unwrap().to_string(),
                    pic: result.get("pic").unwrap().to_string(),
                    points: result.get("points").unwrap().to_string()
                };
                Ok(Response::new(res))
            }
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
struct Claims {
    iss: String,
    aud: String,
    sub: String
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load env vars
    dotenv::dotenv().ok();
    let n = env::var("n").unwrap(); 
    let e = env::var("e").unwrap();
    let iss = env::var("iss").unwrap();
    let aud = env::var("aud").unwrap();

    // Check auth in main so env vars only retrieved once, closure for IDKKKK
    let check_auth = move |req: Request<()>| -> Result<Request<()>, Status> {
        // Get token from request
        let token = match req.metadata().get("authorization") {
            Some(t) => t.to_str().unwrap(),
            None => ""
        };
    
        let mut validation = Validation::new(Algorithm::RS256);
        validation.set_issuer(&[&iss]);
        validation.set_audience(&[&aud]);
        validation.set_required_spec_claims(&["exp", "aud", "iss", "sub"]);
        //validation.sub
        
        let decode_key = DecodingKey::from_rsa_components(&n, &e).unwrap();
        let token_data = decode::<Claims>(&token, &decode_key, &validation);
        if token_data.is_err() {
            match token_data.unwrap_err().kind() {
                ErrorKind::InvalidToken => Err(Status::unauthenticated("Token shape is invalid")),
                ErrorKind::InvalidIssuer => Err(Status::unauthenticated("Issuer is invalid")),
                ErrorKind::InvalidAudience => Err(Status::unauthenticated("Audience is invalid")),
                _ => Err(Status::unauthenticated("Some token error")),
            }
        } else {
            Ok(req)
        }
    };

    let addr = "0.0.0.0:50051".parse().unwrap();
    let pair = MyPairApi::default();

    println!("PairServer listening on {}", addr);

    // Authorization interceptor
    let auth_api = PairApiServer::with_interceptor(pair, check_auth);

    Server::builder()
        .add_service(auth_api)
        .serve(addr)
        .await?;
    Ok(())
}
