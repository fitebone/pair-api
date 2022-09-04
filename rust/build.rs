use tonic_build::compile_protos;

fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load env vars
    compile_protos("proto/pairapi.proto")?;
    Ok(())
}