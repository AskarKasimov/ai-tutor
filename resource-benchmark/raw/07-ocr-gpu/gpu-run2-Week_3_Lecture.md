# Week 3 Lecture


## Слайд 1

## Introduction to Artificial Intelligence

Week 3

## Слайд 2

## Agenda

- ´ What is an Agent
- ´ PEAS
- ´ Properties of the Environment
- ´ Types of Agents

## Слайд 3

## Agents

Mr. Anderson….

<!-- image -->

## Слайд 4

## What is an Agent?

- ´ An agent is anything which:
- ´ Lives in an environment
- ´ Can perceive the environment via its sensors
- ´ Can act upon the environment via its actuators
- ´ Agents also have memory of the current and past perceptions of the environment
- ´ Percept - the current state
- ´ Percept Sequence - the complete history of Percepts

<!-- image -->

## Слайд 5

## Humans as Agents

- ´ Environments
- ´ Home
- ´ School
- ´ Work
- ´ Sensors
- ´ Eyes
- ´ Ears
- ´ Mouth
- ´ Nose
- ´ Skin
- ´ Actuators
- ´ Hands
- ´ Feet
- ´ Speech

<!-- image -->

## Слайд 6

## Rationality

- ´ What is rational at any given time depends upon:
- ´ The performance measure defines success
- ´ The agent's prior knowledge of the environment
- ´ The actions which an agent can perform
- ´ The agent's percept sequence to date
- ´ Definition:
- ´ For Each possible percept sequence, a rational agent should select an action that is expected to maximize its performance measure, given the evidence provided by the percept sequence and whatever built-in knowledge the agent has

<!-- image -->

### Текст изображения 4 (OCR)

IRRATIONAL

## Слайд 7

## Agents are Rational, Not Necessarily Omniscient

- ´ A jester knocks over the king's wine
- ´ "What an idiot!"
- ´ Did the jester make a cartwheel and miss the landing?
- ´ Did the jester see it being poisoned and the king did not?
- ´ Agents need to gather information before making a decision of an action
- ´ "Why did you spill my wine jester?!"
- ´ Note that agents may perceive differently from each other
- ´ Agents have a degree of autonomy
- ´ The King does not control every action of the jester

<!-- image -->

## Слайд 8

## PEAS - Performance Measure, Environment, Actuators, Sensors

<!-- image -->

### Текст изображения 6 (OCR)

Agent
Percepts
Actions

## Слайд 9

<!-- image -->

## PEAS - Performance Measure, Environment, Actuators, Sensors

- ´ Agent Type
- ´ Jester
- ´ Performance measure
- ´ The King is Happy
- ´ Environment
- ´ The Court
- ´ Actuators
- ´ Tumbling
- ´ Juggling
- ´ Joke Telling
- ´ Sensors
- ´ Hearing Laughter
- ´ Seeing Smiles

<!-- image -->

## Слайд 10

## Properties of the Environment (1)

- ´ Fully Observable v. Partially Observable
- ´ Single v. Multiple Agent
- ´ Deterministic v. Stochastic
- ´ Episodic v. Sequential
- ´ Static v. Dynamic
- ´ Discrete v. Continuous
- ´ Known v. Unknown

## Слайд 11

## Properties of the Environment (2)

<!-- image -->

- ´ Fully Observable v. Partially Observable
- ´ Can the agent see everything in the world via its sensors?
- ´ Does it miss elements which are relevant to the decision making process?
- ´ Unobservable for agent without sensors
- ´ Single-agent v. Multiagent
- ´ Is there only one agent within the space?
- ´ Does there need to be some form of competitive or cooperation between agents?
- ´ How do the agents communicate?

## Слайд 12

## Properties of the Environment (3)

- ´ Deterministic v. Stochastic
- ´ Can we determine the next state of the environment given the previous state and the action we are applying into it?
- ´ We can be certain in environments which are fully observable and deterministic
- ´ Episodic v. Sequential
- ´ Episodic environments allow for atomic actions not dependant upon those previous
- ´ Sequential actions are those which the current decision has consequences on future actions

<!-- image -->

## Слайд 13

## Properties of the Environment (4)

- ´ Static v. Dynamic
- ´ Does the environment change as the agent is thinking about its actions?
- ´ Semi-dynamic if the environment does not change as the time passes by but the agent's performance score does
- ´ Discrete v. Continuous
- ´ Are there states in the environment, or does the environment smoothly act over time
- ´ Chess - discrete movements
- ´ Jenga - continuous actions of physics on the blocks
- ´ Known v. Unknown
- ´ Does the designer of the agent have full knowledge of the rules of the environment?

<!-- image -->

## Слайд 14

## The Worst Case

- ´ An environment which is partially observable, multiagent, stochastic, sequential, dynamic, continuous, and unknown
- ´ Ordering in a new restaurant in a new country
- ´ Can't see how they make the food
- ´ Have to deal with the waiter, other customers
- ´ Can't predict when the food will come or how exactly it will taste
- ´ Depending on what I order, I might order more or might order an ambulance
- ´ Maybe they don't have anymore what I would like to order
- ´ Time keeps on ticking
- ´ Don't know the cultural rules

<!-- image -->

<!-- image -->

### Текст изображения 12 (OCR)

顺SoonFatt 发
CHINESE TAKE AWAY

## Слайд 15

## Testing an AI

<!-- image -->

- ´ We do not just want one environment but an environment class
- ´ Run multiple simulations with many different changes to the environment
- ´ Self-driving Car
- ´ Traffic level
- ´ Lights
- ´ Weather
- ´ Collectable Card Game Player
- ´ Against many other decks
- ´ Other player agents

<!-- image -->

## Слайд 16

## Types of Agents

1. Simple reflex agents
2. Model-based reflex agents
3. Goal-based agents
4. Utility-based agents

## Слайд 17

## Simple Reflex Agents

<!-- image -->

### Текст изображения 16 (OCR)

Sensors
What the world
is like now
Environment
Condition-action rules
should do now
What action I
Actuators

## Слайд 18

## Simple Reflex Agents

- ´ Only current percept matters
- ´ Work only in fully observable environment
- ´ Too many condition-action rules may be required

## Слайд 19

## Model-based Reflex Agents

<!-- image -->

### Текст изображения 17 (OCR)

Sensors
State
How the world evolves
What the world
is like now
Environment
What my actions do
Condition-action rules
What action I
should do now
Actuators

## Слайд 20

## Model-based Reflex Agents

- ´ Work better in partially observable environment because of keeping track of the world
- ´ Internal state exists
- ´ In partially observable environment not always possible to answer "What the world is like now?". Sometimes agent tries to guess
- ´ Still, too many condition-action rules may be required
- ´ Not flexible enough

## Слайд 21

## Goal-based Agents

<!-- image -->

### Текст изображения 18 (OCR)

Sensors
State
What the world
How the world evolves
is like now
Environment
What it will be like
What my actions do
if I do action A
What action I
Goals
should do now
Actuators

## Слайд 22

## Goal-based Agents

- ´ More flexible than previous agent types
- ´ Goal replacement does not entail rewriting of all the rules
- ´ May be hard to choose between multiple goals

## Слайд 23

## Utility-based Agents

<!-- image -->

### Текст изображения 19 (OCR)

Sensors
State
What the world
How the world evolves
is like now
What it will be like
What my actions do
if I do action A
onment
How happy I will be
in such a state
What action I
should do now
Actuators

## Слайд 24

## Utility-based Agents

- ´ Allow to maximize the "happiness" of the agent
- ´ Allows to find the tradeoff between the conflicting goals
- ´ More difficult to implement

## Слайд 25

## Learning Agents

- ´ In 1950 Alan Turing proposed to build learning machines (agents) and then to teach them
- ´ Any type of agent (model-based, goal-based, utility-based, etc.) can be built as a learning agent (or not)
- ´ Performance element is what we previously considered as entire agent
- ´ Leaning element is responsible for making improvements
- ´ The learning element receives a feedback from the Critic
- ´ Problem generator is responsible for suggesting actions that will lead to new and informative experiences

## Слайд 26

## Learning Agents

<!-- image -->

### Текст изображения 20 (OCR)

Performance standard
Critic
Sensors
feedback
Environment
changes
Performance
element
element
knowledge
goals
Problem
generator
Actuators

## Слайд 27

## Summary

- ´ What is an Agent
- ´ PEAS
- ´ Properties of the Environment
- ´ Types of Agents

Next week: Expert Systems