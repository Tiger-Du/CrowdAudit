import logging
import os
import re
import time
from datetime import datetime

import cohere
import google.generativeai as genai
import numpy as np
import pandas as pd
import plotly.express as px
import psycopg
import streamlit as st
from groq import Groq
from openai import OpenAI

###############################################################################

# Streamlit Configuration
st.set_page_config(
    page_title="CrowdAudit: Evaluate LLMs", page_icon="favicon.ico", layout="wide"
)

# Custom Internal CSS

hide_streamlit_style = """
                <style>
                div[data-testid="stToolbar"] {
                visibility: hidden;
                height: 0%;
                position: fixed;
                }
                div[data-testid="stDecoration"] {
                visibility: hidden;
                height: 0%;
                position: fixed;
                }
                div[data-testid="stStatusWidget"] {
                visibility: hidden;
                height: 0%;
                position: fixed;
                }
                #MainMenu {
                visibility: hidden;
                height: 0%;
                }
                header {
                visibility: hidden;
                height: 0%;
                }
                footer {
                visibility: hidden;
                height: 0%;
                }
                </style>
                """
st.markdown(hide_streamlit_style, unsafe_allow_html=True)

hide_decoration_bar_style = """
    <style>
        header {visibility: hidden;}
    </style>
"""
st.markdown(hide_decoration_bar_style, unsafe_allow_html=True)

hide_streamlit_style = """
<style>
    #root > div:nth-child(1) > div > div > div > div > section > div {padding-top: 0rem;}
</style>
"""
st.markdown(hide_streamlit_style, unsafe_allow_html=True)

###############################################################################

### Environment Variables

## API Keys

GOOGLE_API_KEY = os.environ["GOOGLE_API_KEY"]
COHERE_API_KEY = os.environ["COHERE_API_KEY"]
GROQ_API_KEY = os.environ["GROQ_API_KEY"]
HF_API_KEY = os.environ["HF_API_KEY"]
OPENROUTER_API_KEY = os.environ["OPENROUTER_API_KEY"]

## Database

host = os.environ["HOST"]
dbname = os.environ["DBNAME"]
port = os.environ["PORT"]
user = os.environ["USER"]
password = os.environ["PASSWORD"]

###############################################################################

# Connect to PostgreSQL Database
connection = psycopg.connect(
    host=host, port=port, dbname=dbname, user=user, password=password
)
cursor = connection.cursor()

# Create Table
query = """
CREATE TABLE IF NOT EXISTS votes (
    model_id       varchar(100)  PRIMARY KEY,
    model_votes    integer       DEFAULT 0,
    model_rounds   integer       DEFAULT 0,
    model_win_rate numeric(3, 2) DEFAULT 0.00);
"""
cursor.execute(query)
connection.commit()

# Initialize Table
query = """
INSERT INTO votes (model_id) VALUES
    ('gemma2-9b-it'),
    ('llama3-8b-8192'),
    ('llama3-70b-8192'),
    ('llama-3.1-8b-instant'),
    ('llama-3.3-70b-versatile'),
    ('command-r-08-2024'),
    ('command-r-plus-08-2024'),
    ('command'),
    ('command-light'),
    ('mixtral-8x7b-32768'),
    ('microsoft/Phi-3-mini-4k-instruct'),
    ('microsoft/Phi-3-medium-128k-instruct')
ON CONFLICT DO NOTHING;
"""
cursor.execute(query)
connection.commit()

# Debug
query = "SELECT * FROM votes;"
cursor.execute(query)
connection.commit()
cursor.fetchone()
for record in cursor:
    print(record)

###############################################################################

### Clients


class GoogleClient:
    def __init__(self, model_ID, name, developer, param_count, url):
        genai.configure(api_key=GOOGLE_API_KEY)

        self.model_ID = model_ID
        self.client = genai.GenerativeModel(self.model_ID)
        self.name = name
        self.developer = developer
        self.param_count = param_count
        self.url = url

    def return_response(self, prompt):
        response = self.client.generate_content(prompt).text

        return response


class CohereClient:
    def __init__(self, model_ID, name, developer, param_count, url):
        self.model_ID = model_ID
        self.client = cohere.Client(api_key=COHERE_API_KEY)
        self.name = name
        self.developer = developer
        self.param_count = param_count
        self.url = url

    def return_response(self, prompt):
        response = self.client.chat(message=prompt, model=self.model_ID).text

        return response


class OpenRouterClient:
    def __init__(self, model_ID, name, developer, param_count, url):
        self.model_ID = model_ID
        self.client = OpenAI(
            base_url="https://openrouter.ai/api/v1",
            api_key=OPENROUTER_API_KEY,
        )
        self.name = name
        self.developer = developer
        self.param_count = param_count
        self.url = url

    def return_response(self, prompt):
        messages = [{"role": "user", "content": prompt}]

        try:
            response = self.client.chat.completions.create(
                model=self.model_ID, messages=messages, max_tokens=100, stream=False
            )
            return response.choices[0].message.content
        except Exception as e:
            logging.warning(e)
            return "Unable to receive response... Please try again!"


# https://console.groq.com/docs/openai
# client = openai.OpenAI(
#     base_url="https://api.groq.com/openai/v1",
#     api_key=os.environ.get("GROQ_API_KEY")
# )
class GroqClient:
    def __init__(self, model_ID, name, developer, param_count, url):
        self.model_ID = model_ID
        self.client = Groq(api_key=GROQ_API_KEY)
        self.name = name
        self.developer = developer
        self.param_count = param_count
        self.url = url

    def return_response(self, prompt):
        chat_completion = self.client.chat.completions.create(
            messages=[
                {"role": "system", "content": "You are a helpful assistant."},
                {
                    "role": "user",
                    "content": prompt,
                },
            ],
            model=self.model_ID,
            temperature=0,
            stream=False,
        )

        response = chat_completion.choices[0].message.content

        return response


class OpenAIClient:
    def __init__(self, model_ID, name, developer, param_count, url):
        self.model_ID = model_ID
        self.client = OpenAI(
            base_url="https://api-inference.huggingface.co/v1/", api_key=HF_API_KEY
        )
        self.name = name
        self.developer = developer
        self.param_count = param_count
        self.url = url

    def return_response(self, prompt):
        messages = [{"role": "user", "content": prompt}]

        response = self.client.chat.completions.create(
            model=self.model_ID, messages=messages, max_tokens=100, stream=False
        )

        return response.choices[0].message.content


###############################################################################

## Random Selection of Two Models

if "rng" not in st.session_state:
    st.session_state["rng"] = np.random.default_rng()


def choose_models():
    model_A, model_B = st.session_state["rng"].choice(
        a=st.session_state["Models List"], size=2, replace=False, shuffle=True
    )

    return model_A, model_B


###############################################################################

# Initialize
if "Models DF" not in st.session_state:
    st.session_state["Models List"] = [
        GroqClient(
            model_ID="gemma2-9b-it",
            name="Gemma 2 9B (IT)",
            developer="Google",
            param_count=9,
            url="https://huggingface.co/google/gemma-2-9b-it",
        ),
        GroqClient(
            model_ID="llama3-8b-8192",
            name="Llama 3 8B",
            developer="Meta",
            param_count=8,
            url="https://huggingface.co/meta-llama/Meta-Llama-3-8B-Instruct",
        ),
        GroqClient(
            model_ID="llama3-70b-8192",
            name="Llama 3 70B",
            developer="Meta",
            param_count=70,
            url="https://huggingface.co/meta-llama/Meta-Llama-3-70B-Instruct",
        ),
        GroqClient(
            model_ID="llama-3.1-8b-instant",
            name="Llama 3.1",
            developer="Meta",
            param_count=8,
            url="https://huggingface.co/meta-llama/Llama-3.1-8B-Instruct",
        ),
        GroqClient(
            name="Llama 3.3",
            model_ID="llama-3.3-70b-versatile",
            developer="Meta",
            param_count=70,
            url="https://huggingface.co/meta-llama/Llama-3.3-70B-Instruct",
        ),
        CohereClient(
            name="Command R 08 2024",
            model_ID="command-r-08-2024",
            developer="Cohere",
            param_count=32,
            url="https://docs.cohere.com/v2/docs/command-r",
        ),
        CohereClient(
            name="Command R+ 08 2024",
            model_ID="command-r-plus-08-2024",
            developer="Cohere",
            param_count=104,
            url="https://docs.cohere.com/v2/docs/command-r-plus",
        ),
        CohereClient(
            name="Command",
            model_ID="command",
            developer="Cohere",
            param_count=52,
            url="https://docs.cohere.com/v2/docs/command-beta",
        ),
        CohereClient(
            name="Command Light",
            model_ID="command-light",
            developer="Cohere",
            param_count=6,
            url="https://docs.cohere.com/v2/docs/command-beta",
        ),
        GroqClient(
            name="Mixtral-8x7B",
            model_ID="mixtral-8x7b-32768",
            developer="Mistral",
            param_count=47,
            url="https://huggingface.co/mistralai/Mixtral-8x7B-Instruct-v0.1",
        ),
        OpenAIClient(
            name="Phi-3-Mini-4K-Instruct",
            model_ID="microsoft/Phi-3-mini-4k-instruct",
            developer="Microsoft",
            param_count=4,
            url="https://huggingface.co/microsoft/Phi-3-mini-4k-instruct",
        ),
        OpenRouterClient(
            name="Phi-3-Medium-128K-Instruct",
            model_ID="microsoft/Phi-3-medium-128k-instruct",
            developer="Microsoft",
            param_count=14,
            url="https://huggingface.co/microsoft/Phi-3-medium-128k-instruct",
        ),
    ]

    st.session_state["Models DF"] = pd.DataFrame(
        data=[
            (
                model.name,
                model.model_ID,
                model.developer,
                model.param_count,
                model.client,
                model.url,
            )
            for model in st.session_state["Models List"]
        ],
        columns=["Name", "ID", "Developer", "Parameter Count", "Client", "Website"],
    )

    st.session_state["Models DF"] = pd.merge(
        left=st.session_state["Models DF"],
        right=pd.read_sql(sql="SELECT * FROM votes", con=connection),
        left_on="ID",
        right_on="model_id",
    )

    # Initialize Model A and Model B
    st.session_state["Model A"], st.session_state["Model B"] = choose_models()

###############################################################################

# Debugging

model_A, model_B = st.session_state["Model A"], st.session_state["Model B"]
print(model_A)
print(model_B)

###############################################################################


## Callback for Button
def cast_vote(model_name: str):
    ## Cast Vote in Database

    if model_name != "Tie":
        query = """UPDATE votes
            SET model_votes = model_votes + 1
            WHERE model_id = %(model_id)s;"""

        print(st.session_state["Model A"].model_ID)

        # Vote for Model A
        if model_name == "A":
            cursor.execute(query, {"model_id": st.session_state["Model A"].model_ID})
            connection.commit()
        # Vote for Model B
        elif model_name == "B":
            cursor.execute(query, {"model_id": st.session_state["Model B"].model_ID})
            connection.commit()

    query = """UPDATE votes
            SET model_rounds = model_rounds + 1
            WHERE model_id = %(model_id)s;"""
    for model in [st.session_state["Model A"], st.session_state["Model B"]]:
        cursor.execute(query, {"model_id": model.model_ID})
        connection.commit()
        print(model.name)

    query = """UPDATE votes
            SET model_win_rate = model_votes / model_rounds
            WHERE model_id = %(model_id)s;"""
    for model in [st.session_state["Model A"], st.session_state["Model B"]]:
        cursor.execute(query, {"model_id": model.model_ID})
        connection.commit()
        print(model.name)

    ###########################################################################

    ## Toast

    if model_name == "A":
        message = "You voted for __Model A__!"
    elif model_name == "Tie":
        message = "You voted for a __tie__!"
    elif model_name == "B":
        message = "You voted for __Model B__!"

    st.toast(
        message
        + f"\n\n__Model A__: {st.session_state["Model A"].name}\n\n__Model B__: {st.session_state["Model B"].name}"
    )

    ###########################################################################

    st.session_state["Models DF"].set_index("ID", inplace=True)
    st.session_state["Models DF"].index.names = ["model_id"]

    st.session_state["Models DF"].update(
        pd.read_sql(sql="SELECT model_id, model_votes FROM votes", con=connection)
    )

    st.session_state["Models DF"].index.names = ["ID"]
    st.session_state["Models DF"].reset_index(inplace=True)

    ###########################################################################

    # Reset
    st.session_state["Model A Response"] = MODEL_RESPONSE_PLACEHOLDER
    st.session_state["Model B Response"] = MODEL_RESPONSE_PLACEHOLDER
    st.session_state["prompt_submitted"] = False
    st.session_state["text_input_disabled"] = False
    st.session_state["Model C Response"] = MODEL_C_RESPONSE_PLACEHOLDER

    print(st.session_state["Models DF"].columns)

    # Reorder
    st.session_state["Models DF"].sort_values(
        by="model_votes", axis="index", inplace=True
    )

    # Re-Choose Models
    st.session_state["Model A"], st.session_state["Model B"] = choose_models()


###############################################################################


def stream(response, interval=0.04):
    for word in response.split(" "):
        yield word + " "
        time.sleep(interval)


def is_empty_prompt(prompt):
    return (
        prompt is None
        or prompt.isspace()
        or prompt == ""
        or not isinstance(prompt, str)
    )


def submit_prompt():
    st.session_state["Model A Response"] = st.session_state["Model A"].return_response(
        st.session_state["Your Prompt"]
    )
    st.session_state["Model B Response"] = st.session_state["Model B"].return_response(
        st.session_state["Your Prompt"]
    )

    st.session_state["prompt_submitted"] = False
    st.session_state["text_input_disabled"] = True


def is_disabled_button():
    return (
        st.session_state["Model A Response"] == MODEL_RESPONSE_PLACEHOLDER
        and st.session_state["Model B Response"] == MODEL_RESPONSE_PLACEHOLDER
    )


###############################################################################

# Title
st.html('<h1 style="text-align: center;">CrowdAudit</h1>')

###############################################################################

# Tabs
evaluation_tab, explorer_tab, models_tab = st.tabs(
    ["__Evaluation__", "__Explorer__", "__Models__"]
)

###############################################################################

with evaluation_tab:
    # Initialize
    MODEL_RESPONSE_PLACEHOLDER = ":gray[_Listening for your prompt..._]"
    MODEL_C_RESPONSE_PLACEHOLDER = "Nothing here yet! Submit a prompt first."
    if "Prompt" not in st.session_state:
        st.session_state["Prompt"] = ""
    if "Model A Response" not in st.session_state:
        st.session_state["Model A Response"] = MODEL_RESPONSE_PLACEHOLDER
    if "Model B Response" not in st.session_state:
        st.session_state["Model B Response"] = MODEL_RESPONSE_PLACEHOLDER
    if "prompt_submitted" not in st.session_state:
        st.session_state["prompt_submitted"] = False
    if "text_input_disabled" not in st.session_state:
        st.session_state["text_input_disabled"] = False

    ###########################################################################

    columns = st.columns([1, 1.2, 1])  # The middle column is the widest.

    # Vertical Spacing
    st.markdown("")
    st.markdown("")

    prompt_columns = st.columns([9, 1])

    with prompt_columns[0]:
        st.session_state["Prompt"] = st.text_input(
            label="Your Prompt",
            # value=st.session_state["Prompt"],
            placeholder="Enter your prompt here...",
            key="Your Prompt",
            on_change=submit_prompt,
            label_visibility="collapsed",
            disabled=st.session_state["text_input_disabled"],
        )
    with prompt_columns[1]:
        st.button("Submit", use_container_width=True, type="primary", key=".")

    ###########################################################################

    # Column for Model A
    with columns[0]:
        with st.container(height=375, border=True):
            st.subheader("Model A", anchor=False, divider="red")

            ###################################################################

            if is_empty_prompt(st.session_state["Prompt"]):
                st.markdown(MODEL_RESPONSE_PLACEHOLDER)
            else:
                if st.session_state["prompt_submitted"]:
                    st.markdown(st.session_state["Model A Response"])
                else:
                    st.write_stream(stream(st.session_state["Model A Response"]))

    ###########################################################################

    # Column for Evaluation
    with columns[1]:
        with st.container(height=375, border=True):
            st.subheader("Your Evaluation", anchor=False, divider="orange")

            st.markdown("Which model has the better response to your prompt?")

            ###################################################################

            with st.container(border=False):
                inner_columns = st.columns(3)

                # Vote for Model A
                with inner_columns[0]:
                    pressed_A = st.button(
                        "Model A",
                        key="Model A Button",
                        on_click=cast_vote,
                        args=("A",),
                        use_container_width=True,
                        type="primary",
                        disabled=is_disabled_button(),
                    )

                # Vote for a Tie
                with inner_columns[1]:
                    pressed_tie = st.button(
                        "Tie",
                        on_click=cast_vote,
                        args=("Tie",),
                        use_container_width=True,
                        type="primary",
                        disabled=is_disabled_button(),
                    )

                # Vote for Model B
                with inner_columns[2]:
                    pressed_B = st.button(
                        "Model B",
                        key="Model B Button",
                        on_click=cast_vote,
                        args=("B",),
                        use_container_width=True,
                        type="primary",
                        disabled=is_disabled_button(),
                    )

            ###########################################################################

            enable_LLM_assistance = st.toggle(
                label="Enable LLM Assistance",
                help="Enable a third LLM to analyze the responses from Model A and Model B and point out relevant excerpts.",
            )

            if enable_LLM_assistance:
                # pass

                if "Model C Response" not in st.session_state:
                    st.session_state["Model C Response"] = (
                        "Nothing here yet! Enter a prompt first."
                    )

                if (
                    st.session_state["Model A Response"] != ""
                    and st.session_state["Model A Response"]
                    != MODEL_RESPONSE_PLACEHOLDER
                    and st.session_state["Model B Response"] != ""
                    and st.session_state["Model B Response"]
                    != MODEL_RESPONSE_PLACEHOLDER
                ):
                    model_C_prompt = f"""What do you think of the following text from 
                                Model A in comparison to Model B? Your first sentence,
                                followed by a newline, should indicate which has
                                better (1) coherence and (2) relevance. Quote
                                specific words and phrases to support your
                                explanation.
                                Model A: {st.session_state["Model A Response"]}. Model
                                B: {st.session_state["Model B Response"]}"""

                    model_C = GoogleClient(
                        name="Gemini 1.5 Flash",
                        model_ID="gemini-1.5-flash",
                        developer="Google",
                        param_count=0,
                        url="https://ai.google.dev/gemini-api/docs/models/gemini",
                    )

                    st.session_state["Model C Response"] = model_C.return_response(
                        model_C_prompt
                    )

                    # Add highlighted annotations.
                    quote_list = re.findall(
                        r'"(.+?)"', st.session_state["Model C Response"]
                    )
                    for quote in quote_list:
                        st.session_state["Model C Response"] = st.session_state[
                            "Model C Response"
                        ].replace(
                            '"' + quote + '"', ':orange-background["' + quote + '"]'
                        )

                with st.expander(label="Response from LLM Assistant", expanded=False):
                    st.markdown(st.session_state["Model C Response"])

    ###########################################################################

    # Column for Model B
    with columns[2]:
        with st.container(height=375, border=True):
            st.subheader("Model B", anchor=False, divider="blue")

            ###################################################################

            if is_empty_prompt(st.session_state["Prompt"]):
                st.markdown(MODEL_RESPONSE_PLACEHOLDER)
            else:
                if st.session_state["prompt_submitted"]:
                    st.markdown(st.session_state["Model B Response"])
                else:
                    st.write_stream(stream(st.session_state["Model B Response"]))
                    st.session_state["prompt_submitted"] = True


###############################################################################

with explorer_tab:
    # Export Data as CSV
    st.download_button(
        label="Export Data as CSV",
        data=st.session_state["Models DF"]
        .drop("Client", axis="columns")
        .to_csv()
        .encode("utf-8"),
        file_name=f"CrowdAudit_{datetime.now()}.csv",
        mime="text/csv",
        type="primary",
    )

    ###########################################################################

    st.subheader("Votes for All Models", anchor=False)

    data = st.session_state["Models DF"][
        ["Name", "model_votes", "Parameter Count"]
    ].sort_values(by="model_votes", axis="index", ascending=False)

    data = pd.merge(
        left=st.session_state["Models DF"][["ID", "Name", "Parameter Count"]],
        right=pd.read_sql(
            sql="SELECT model_id, model_votes FROM votes", con=connection
        ),
        left_on="ID",
        right_on="model_id",
    )

    print(data)

    # Plot
    fig = px.bar(data, x="model_votes", y="Name", color="model_votes", orientation="h")
    fig.update_layout(yaxis={"categoryorder": "total ascending"})

    st.plotly_chart(fig, config=dict(displayModeBar=False))

    # Plot
    if data["model_votes"].sum() != 0:
        st.markdown("#### Win Rates by Parameter Count")

        fig = px.scatter(
            data,
            x="Parameter Count",
            y="model_votes",
            color="model_votes",
            hover_name="Name",
            trendline="ols",
        )
        fig.update_traces(marker={"size": 15})

        st.plotly_chart(fig, config=dict(displayModeBar=False))

###############################################################################

with models_tab:
    st.dataframe(
        st.session_state["Models DF"][
            ["Name", "ID", "Parameter Count", "Developer", "Website"]
        ].sort_values(by="Name"),
        hide_index=True,
        height=450,
        use_container_width=True,
        column_config={
            "Name": st.column_config.TextColumn(label="Name", width="medium"),
            "ID": st.column_config.TextColumn(label="ID", width="medium"),
            "Parameter Count": st.column_config.TextColumn(
                label="Parameter Count (B)", width="small"
            ),
            "Developer": st.column_config.TextColumn(label="Developer", width="small"),
            "Website": st.column_config.LinkColumn(label="Website", width="medium"),
        },
    )

###############################################################################

st.markdown("")
st.markdown("")
st.markdown("")
st.markdown("")
st.markdown("")

st.divider()

st.markdown(
    "Created by Tiger Du | __GitHub Repository__: [github.com/Tiger-Du/CrowdAudit](https://github.com/Tiger-Du/CrowdAudit)"
)

###############################################################################

# CSS to remove the blank space at the bottom.
st.markdown(
    """
    <style>
    
           /* Remove blank space at top and bottom */ 
           .block-container {
               padding-top: 0rem;
               padding-bottom: 0rem;
            }
           
           /* Remove blank space at the center canvas */ 
           .st-emotion-cache-z5fcl4 {
               position: relative;
               top: -62px;
               }
           
           /* Make the toolbar transparent and the content below it clickable */ 
           .st-emotion-cache-18ni7ap {
               pointer-events: none;
               background: rgb(255 255 255 / 0%)
               }
           .st-emotion-cache-zq5wmm {
               pointer-events: auto;
               background: rgb(255 255 255);
               border-radius: 5px;
               }
    </style>
    """,
    unsafe_allow_html=True,
)
