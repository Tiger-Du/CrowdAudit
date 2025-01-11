## CrowdAudit

[![Deploy to Amazon Web Services](https://github.com/Tiger-Du/CrowdAudit/actions/workflows/deploy_aws_ecs.yml/badge.svg)](https://github.com/Tiger-Du/CrowdAudit/actions/workflows/deploy_aws_ecs.yml)

CrowdAudit is a full-stack application that enables evaluation of generative AI.

CrowdAudit can be used as:

- A local application in Python
- A Docker container
- A hosted application on AWS at [crowdaudit.org](https://crowdaudit.org)

CrowdAudit supports models from many providers, including Hugging Face, Groq, and Cohere.

## Deploying on AWS

CrowdAudit can be deployed on AWS with Terraform.

The provisioned infrastructure is an Elastic Container Service (ECS) cluster with an Auto Scaling group of EC2 instances behind an Application Load Balancer (ALB) for high availability.

<details><summary><b>Architecture</b></summary>
<img src=diagram.png>
</details>

## Repository

```code
├── .aws                       # AWS-specific files
│   └── task-definition.json   # Task definition for Amazon ECS
├── .github                    # GitHub-specific files
│   └── .workflows             # Workflow files for GitHub Actions
│       └── deploy_aws_ecs.yml # Workflow file for deploying CrowdAudit to AWS
├── src                        # Source code for CrowdAudit
│   ├── .streamlit             # Streamlit-specific files
│   │   └── config.toml        # Configuration file for Streamlit
│   ├── app.py                 # Streamlit application
│   └── favicon.ico            # Favicon
├── .dockerignore              # Exclude files from the Docker build context
├── .gitignore                 # Exclude files from git management
├── Dockerfile                 # Script to build Docker images of CrowdAudit
├── LICENSE                    # License for CrowdAudit
├── README.md                  # Description of CrowdAudit
└── requirements.txt           # List of Python packages that CrowdAudit depends on
```
