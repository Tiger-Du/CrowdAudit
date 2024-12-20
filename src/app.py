from datetime import datetime
import os
import re
import time

import cohere
import google.generativeai as genai
from groq import Groq
import numpy as np
import pandas as pd
import streamlit as st

###############################################################################

### API Keys and Clients

GOOGLE_API_KEY = os.environ["GOOGLE_API_KEY"]
COHERE_API_KEY = os.environ["COHERE_API_KEY"]
GROQ_API_KEY = os.environ["GROQ_API_KEY"]

###############################################################################

class GoogleClient():
    def __init__(self, model_ID):
        genai.configure(api_key=GOOGLE_API_KEY)
        self.model_ID = model_ID
        self.client = genai.GenerativeModel(self.model_ID)
    
    def return_response(self, prompt):
        response = self.client.generate_content(prompt).text

        return response

class CohereClient():
    def __init__(self, model_ID):
        self.model_ID = model_ID
        self.client = cohere.Client(api_key=COHERE_API_KEY)
    
    def return_response(self, prompt):
        response = self.client.chat(message=prompt, model=self.model_ID).text

        return response

class GroqClient():
    def __init__(self, model_ID):
        self.model_ID = model_ID
        self.client = Groq(api_key=GROQ_API_KEY)
    
    def return_response(self, prompt):
        chat_completion = self.client.chat.completions.create(
            messages=[
                {
                    "role": "system",
                    "content": "You are a helpful assistant."
                },
                {
                    "role": "user",
                    "content": prompt,
                }
            ],
            model=self.model_ID,
            temperature=0,
            stream=False
        )

        response = chat_completion.choices[0].message.content

        return response

###############################################################################

## Random Selection of Two Models

if 'rng' not in st.session_state:
    random_seed = 0
    st.session_state['rng'] = np.random.default_rng(random_seed)

def choose_models():
    model_A, model_B = st.session_state['rng'].choice(a=st.session_state["df"]['Client'],
                                  size=2,
                                  replace=False,
                                  shuffle=True)
    
    return model_A, model_B

###############################################################################

# Initialize
if "df" not in st.session_state:
    models_list = [["Gemini 1.5 Flash", 'gemini-1.5-flash', "Google", GoogleClient('gemini-1.5-flash'), 'https://ai.google.dev/gemini-api/docs/models/gemini'],
                    ["Gemma 1.1 7B (IT)", 'gemma-7b-it', "Google", GroqClient('gemma-7b-it'), 'https://huggingface.co/google/gemma-1.1-7b-it'],
                    ["Gemma 2 9B (IT)", 'gemma2-9b-it', "Google", GroqClient('gemma2-9b-it'), 'https://huggingface.co/google/gemma-2-9b-it'],
                    ['Llama 3 8B', 'llama3-8b-8192', 'Meta', GroqClient('llama3-8b-8192'), 'https://huggingface.co/meta-llama/Meta-Llama-3-8B-Instruct'],
                    ["Llama 3 70B", 'llama3-70b-8192', "Meta", GroqClient('llama3-70b-8192'), 'https://huggingface.co/meta-llama/Meta-Llama-3-70B-Instruct'],
                    ["Llama 3.1", 'llama-3.1-8b-instant', "Meta", GroqClient('llama-3.1-8b-instant'), 'https://huggingface.co/meta-llama/Llama-3.1-8B-Instruct'], 
                    ["Llama 3.3", 'llama-3.3-70b-versatile', "Meta", GroqClient('llama-3.3-70b-versatile'), 'https://huggingface.co/meta-llama/Llama-3.3-70B-Instruct'],
                    ["Command R 08 2024", 'command-r-08-2024', "Cohere", CohereClient('command-r-08-2024'), 'https://docs.cohere.com/v2/docs/command-r'],
                    ["Command R+ 08 2024", 'command-r-plus-08-2024', "Cohere", CohereClient('command-r-plus-08-2024'), 'https://docs.cohere.com/v2/docs/command-r-plus'],
                    ['Command', 'command', "Cohere", CohereClient('command'), 'https://docs.cohere.com/v2/docs/command-beta'],
                    ['Command Light', 'command-light', "Cohere", CohereClient('command-light'), 'https://docs.cohere.com/v2/docs/command-beta'],
                    ['Mixtral-8x7B', 'mixtral-8x7b-32768', 'Mistral', GroqClient('mixtral-8x7b-32768'), 'https://huggingface.co/mistralai/Mixtral-8x7B-Instruct-v0.1']]

    all_df = pd.DataFrame(data=models_list, columns=['Name', 'ID', 'Developer', 'Client', 'Website'])
    all_df['Votes'] = 0

    st.session_state["df"] = all_df

    st.session_state["Model A"], st.session_state["Model B"] = choose_models()

model_A, model_B = st.session_state["Model A"], st.session_state["Model B"]
print(model_A)
print(model_B)

###############################################################################

# Callback
def increment(model_name: str):
    if model_name != "Tie":
        st.session_state["df"].loc[st.session_state["df"]['Client'] == st.session_state["Model A"], 'Votes'] += 1

    if model_name == "A":
        st.toast("You voted for __Model A__! Resetting...")
    elif model_name == "Tie":
        st.toast("You voted for a __tie__! Resetting...")
    elif model_name == "B":
        st.toast("You voted for __Model B__! Resetting...")
    
    # Reset
    st.session_state["Model A Response"] = ":gray[_Listening for your prompt..._]"
    st.session_state["Model B Response"] = ":gray[_Listening for your prompt..._]"
    st.session_state["Your Prompt"] = ""

    # Reset Models
    st.session_state["Model A"], st.session_state["Model B"] = choose_models()

def stream(response):
    for word in response.split(" "):
        yield word + " "
        time.sleep(0.02)

###############################################################################

st.set_page_config(page_title="CrowdAudit: Evaluate LLMs",
                   page_icon="favicon.ico",
                   layout="wide")

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

hide_decoration_bar_style = '''
    <style>
        header {visibility: hidden;}
    </style>
'''
st.markdown(hide_decoration_bar_style, unsafe_allow_html=True)

hide_streamlit_style = """
<style>
    #root > div:nth-child(1) > div > div > div > div > section > div {padding-top: 0rem;}
</style>

"""
st.markdown(hide_streamlit_style, unsafe_allow_html=True)

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

###############################################################################

st.html('<h1 style="text-align: center;">CrowdAudit</h1>')

###############################################################################

evaluation_tab, explorer_tab, models_tab = st.tabs(["__Evaluation__",
                                                    "__Explorer__",
                                                    "__Models__"])

###############################################################################

with evaluation_tab:
    if "Model A Response" not in st.session_state:
        st.session_state["Model A Response"] = ":gray[_Listening for your prompt..._]"
    if "Model B Response" not in st.session_state:
        st.session_state["Model B Response"] = ":gray[_Listening for your prompt..._]"

    model_A_response = ""
    model_B_response = ""

    columns = st.columns(3)

    prompt = st.text_input(label="Your Prompt",
                           placeholder="Enter your prompt here...",
                           key="Your Prompt")

    ###########################################################################

    # Model A
    with columns[0]:
        with st.container(height=300, border=True):
            st.subheader("Model A", anchor=False, divider='red')

            if prompt is None or prompt.isspace() or prompt == "" or not isinstance(prompt, str):
                response = st.session_state["Model A Response"]

                st.markdown(st.session_state["Model A Response"])

                response = ""

                model_A_response = response
            else:
                st.session_state["Model A Response"] = model_A.return_response(prompt)

                response = st.session_state["Model A Response"]

                model_A_response = response

                st.write_stream(stream(st.session_state["Model A Response"]))

    ###########################################################################

    with columns[1]:
        with st.container(height=300, border=True):
            st.subheader("Your Evaluation", anchor=False, divider='orange')

            st.markdown("Which model has the better response to your prompt?")

            with st.container(border=False):
                inner_columns = st.columns(3)
                with inner_columns[0]:
                    pressed_A_bias = st.button("Model A",
                                            key="A_bias",
                                            on_click=increment,
                                            args=("A", ),
                                            use_container_width=True,
                                            type='primary')
                with inner_columns[1]:
                    pressed_tie = st.button("Tie",
                                  on_click=increment,
                                  args=("Tie", ),
                                  use_container_width=True,
                                  type='primary')
                with inner_columns[2]:
                    pressed_B_bias = st.button("Model B",
                                            key="B_bias",
                                            on_click=increment,
                                            args=("B", ),
                                            use_container_width=True,
                                            type='primary')

###############################################################################

    with columns[2]:
        with st.container(height=300, border=True):
            st.subheader("Model B", anchor=False, divider='blue')

            if prompt is None or prompt.isspace() or prompt == "" or not isinstance(prompt, str):

                st.markdown(st.session_state["Model B Response"])

                response = ""

                model_B_response = response
            else:
                response = model_B.return_response(prompt)

                model_B_response = response
            
                st.write_stream(stream(response))

###############################################################################

    enable_LLM_assistance = st.toggle(label="Enable LLM Assistance",
                                    help="Enable a third LLM to analyze the responses from Model A and Model B and point out relevant excerpts.")

    if enable_LLM_assistance:
        if model_A_response != "" and model_B_response != "":
            prompt = f"What do you think of the following text from Model A in comparsion to Model B? Your first sentence, followed by a newline, should indicate which has better (1) coherence and (2) relevance. Quote specific words and phrases to support your explanation. Model A: {model_A_response}. Model B: {model_B_response}"

            model_C = choose_models()[0]

            response = model_C.return_response(prompt)

            quote_list = re.findall(r'"(.+?)"', response)

            for quote in quote_list:
                response = response.replace('"' + quote + '"', ':orange-background["' + quote + '"]')
        else:
            response = "Nothing here yet! Enter a prompt above first."

        with st.expander(label="Response from LLM Assistant", expanded=False):
            st.markdown(response)

    ###########################################################################

with explorer_tab:

    st.download_button(label="Export Data as CSV",
                       data=st.session_state['df'].drop('Client', axis='columns').to_csv().encode('utf-8'),
                       file_name=f"CrowdAudit_{datetime.now()}.csv",
                       mime="text/csv",
                       type='primary')

    with st.expander(label="Votes", expanded=True):
        st.markdown("#### Votes for All Models")
        st.bar_chart(data=st.session_state['df'][['Name', 'Votes']],
                     x='Name',
                     y='Votes',
                     x_label="Votes",
                     y_label="Model",
                     horizontal=True)

###############################################################################

with models_tab:
    st.dataframe(st.session_state['df'][['Name', 'ID', 'Developer', 'Website']],
                 hide_index=True,
                 use_container_width=True,
                 column_config={'Name': st.column_config.TextColumn(label="Name", width='medium'),
                                'ID': st.column_config.TextColumn(label="ID", width='medium'),
                                'Developer': st.column_config.TextColumn(label="Developer", width='small'),
                                "Website": st.column_config.LinkColumn(label="Website", width='medium')})

###############################################################################

st.divider()

st.markdown("Created by Tiger Du | __GitHub Repository__: [github.com/Tiger-Du/CrowdAudit](https://github.com/Tiger-Du/CrowdAudit)")

# Remove the blank space at the bottom.
st.markdown("""
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
    """, unsafe_allow_html=True)
