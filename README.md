### CrowdAudit

[![Deploy to Amazon Web Services](https://github.com/Tiger-Du/CrowdAudit/actions/workflows/deploy_aws_ecs.yml/badge.svg)](https://github.com/Tiger-Du/CrowdAudit/actions/workflows/deploy_aws_ecs.yml)

CrowdAudit is a full-stack application that enables evaluation of generative AI.

CrowdAudit can be used as:

- A Streamlit application
- A Docker image
- A hosted application on AWS at [crowdaudit.org](https://crowdaudit.org)

### Deploying on AWS

CrowdAudit can be deployed on AWS with Terraform.

The infrastructure is an Elastic Container Service (ECS) cluster with an Auto Scaling group of EC2 instances behind an Application Load Balancer (ALB).
