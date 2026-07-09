data "terraform_remote_state" "data" {
  backend = "local"
  config = {
    path = "../data/terraform.tfstate"
  }
}

module "cluster" {
  source        = "../../../modules/region-cluster"
  name          = "courtside-us"
  region        = "nyc3"
  vpc_id        = data.terraform_remote_state.data.outputs.vpc_id
  db_cluster_id = data.terraform_remote_state.data.outputs.db_cluster_id
}
