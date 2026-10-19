# Tutorial 2


## Слайд 1

## Logic

Darko Bozhinoski, Ph.D. in Computer Science

## Слайд 2

## Lecture Objectives

- ❖ Propositional Logic
- ❖ First-Order Logic

## Слайд 3

## PROPOSITIONAL LOGIC

- Sentences considered in propositional logic are not arbitrary sentences but are the ones that are either true or false, but not both. This kind of sentences are called propositions. By reading a sentence and basing your conclusion directly from the facts mentioned in the sentence only, you can conclude the truth or falsity of the statement.

## Слайд 4

## Examples ?? (Discussion)

## Слайд 5

- ❖ Grass is green
- ❖ 2 + 5 = 5
- ❖ x + 3 = 3
- ❖ x +3 &gt; 5
- ❖ John is in the classroom.
- ❖ Mary is enrolled in 270A.

## Examples

## Слайд 6

- A statement is a sentence that is either true or false, but not both.
- Statements:
- It is raining.
- I am carrying an umbrella.
- Non-statements:
- a. (Question): Why are you late?
- b. (Command): Open the door.
- c. (Wish): If only I had studied a little harder…
- d. (Something Vague): x + y = 4.

## Слайд 7

## COMPOUND STATEMENTS

A compound statement is one with two or more simple statements as parts - components. A component of a compound is any whole statement that is part of a larger statement.

Simple sentences which are true or false are basic propositions.

Complex sentences are constructed from basic propositions by combining them with connectives.

- propositions and connectives are the basic elements of propositional logic. Five basic connectives here:

| NOT            | or ~   |
|----------------|--------|
| AND            |        |
| OR             |        |
| IF_THEN        |        |
| IF and ONLY IF |        |

## Слайд 8

## Logical Operators

- Binary operators
- Conjunction - "and".
- Disjunction - "or".
- Unary operator
- Negation - "not".
- Other operators
- XOR - "exclusive or"
- NAND - "not both"
- NOR - "neither"

## Слайд 9

## Elements of Propositional Logic

## Simple sentences.

A simple statement is one that does not contain any other statement as a part. We will use the lower-case letters, p, q, r, ..., as symbols for simple statements.

If p or q, then r

Therefore, if not r, then not p and not q.

E.g. If Jane is a math major or Jane is a computer science major, then Jane will take Math 150.

Jane is a computer science major.

Therefore, Jane will take Math 150.

## Слайд 10

## Examples

- P means "It is hot."
- Q means "It is humid."
- R means "It is raining."
- (P ∧ Q) → R

"If it is hot and humid, then it is raining"

- Q → P

"If it is humid, then it is hot"

- A better way:

```
Hot = ``It is hot'''
    Humid = ``It is humid'''

```

Raining = "It is raining"

## Слайд 11

## Translation into Propositional Logic

- If it rains, then the game will be cancelled.
- If the game is cancelled, then we clean house.
- Can we conclude?
- If it rains, then we clean house.
- p = it rains, q = game cancelled r = we clean house.
- If p then q. not p or q
- If q then r. not q or r
- if p then r. not p or r (resolution)

## Слайд 12

## Translation into Propositional Logic

- ❖ Basic statements
- ❖ Compound statements
- ➢p ∧ q = "It is raining and I am carrying an umbrella."
- ➢p ∨ q = " It is raining or I am carrying an umbrella."
- ➢~p = "It is not raining."

```
>> p = "It is rainin."
      >> q = "I am carrying an umbrella."
```

## Слайд 13

## TRUTH VALUES

Each simple statement comprising a compound statement is a statement in its own right, that is it has a truth value, that is it can be true or false but not both.

The truth values of each individual statements affect the truth value of the compound statement of which they are part of.

## NEGATION

If p is a statement variable, the negation of p is "not p" or "It is not the case that p" and is denoted as ~p. It has opposite truth values as p. Wherever p is true, ~p is false and vice versa. The truth values for negation summarized in a truth table are

| p   | ~p   |
|-----|------|
| T   | F    |
| F   | T    |

## Слайд 14

## LOGICAL AND

If p and q are statement variables, the conjunction of p and q is "p and q" denoted p q. It is true when, and only when, both p and q are true. If either p or q is false, or if both are false, p q is false. The truth values for and summarized in a truth table are

| p   | q   | p q   |
|-----|-----|-------|
| T   | T   | T     |
| T   | F   | F     |
| F   | T   | F     |
| F   | F   | F     |

## Слайд 15

## LOGICAL OR

If p and q are statement variables, the disjunction of p and q is "p or q" denoted p q. It is true when at least one of p or q is true and is false only when both p and q are false. The truth values for p or q summarized in a truth table are

| p   | q   | p q   |
|-----|-----|-------|
| T T | F T | T T   |
| F   | T   | T     |

F

F

F

## Слайд 16

## TRUTH TABLES

The general rule for constructing truth tables is:

1. List all the variables mentioned in the statement in separate columns and then list all the combinations of truth values for them.
2. Add columns for operations mentioned in the statement with all the combinations of truth values for them starting from inside out the parenthesis if there exist any.
3. Evaluate the final result

Exercise. Construct truth table for:

```
- p OR (p AND q)
```

## Слайд 17

## TRUTH TABLES

The general rule for constructing truth tables is:

1. List all the variables mentioned in the statement in separate columns and then list all the combinations of truth values for them.
2. Add columns for operations mentioned in the statement with all the combinations of truth values for them starting from inside out the parenthesis if there exist any.
3. Evaluate the final result

Exercise. Construct truth table for:

```
- p OR (p AND q)
```

Result: https://web.stanford.edu/class/cs103/tools/truth-table-tool/

## Слайд 18

## Logical Equivalence

- ❖ Two statements are said to be logically equivalent if and only if, they've identical truth values for each possible substitution of statements for their statement variables.
- ❖ It is denoted as P 三

## Follow this procedure:

- ❖ Construct the truth table for P
- ❖ Construct the truth table for Q using the same statement variables for identical component statements.
- ❖ Check each combination of truth values of the statement variables:
- a. If in each row the truth value of P is the same as the truth value of Q, then P and Q are logically equivalent.
- b. Otherwise, P and Q are not logically equivalent.

## Слайд 19

Are (p ∧ q) ∨ (~p ∧ ~q) and (p ∨ ~q) ∧ (~p ∨ q) logically equivalent?

## Слайд 20

<!-- formula-not-decoded -->

| p   | q   | (p ∧ q) ∨ (~p ∧ ~q)   | (p ∨ ~q) ∧ (~p ∨ q)   |
|-----|-----|-----------------------|-----------------------|
| T   | T   | T                     | T                     |
| T   | F   | F                     | F                     |
| F   | T   | F                     | F                     |
| F   | F   | T                     | T                     |

## Слайд 21

## De Morgan's law

- ·¬(p q) ¬p V ¬q the negation of and is logically equivalent to the or of the negated components
- ·¬(p V q) ¬p ¬q the negation of oris logically equivalent to the and of the negated components

## Слайд 22

## Tautology and Contradiction

A tautology is a logical expression that is true regardless of the values of its variables. Thus, for a tautology, the value of the expression is TRUE in every row of the truth table. The column of a tautology in a truth table contains only T's.

## Example??

A proposition which is always false is called a contradiction. The column of a contradiction in a truth table contains only F's.

## Example??

## Слайд 23

## Tautology and Contradiction

A tautology is a logical expression that is true regardless of the values of its variables. Thus, for a tautology, the value of the expression is TRUE in every row of the truth table. The column of a tautology in a truth table contains only T's.

Example: P V (NOT P)

A proposition which is always false is called a contradiction. The column of a contradiction in a truth table contains only F's.

Example: P ∧ (NOT P)

## Слайд 24

## Conditional statements

If p and q are statement variables, the conditional of q by p is "if p then q" or "p implies q" and is denoted by .

It is false when p is true and q is false.

A conditional statement is vacuously true or true by default if its hypothesis is false.

| p   | q   | p   | q   |
|-----|-----|-----|-----|
| T   | T   | T   |     |
| T   | F   | F   |     |
| F   | F   | T   |     |
| F   | T   | T   |     |

## Слайд 25

$$p \rightarrow q \equiv \neg p \ V q$$

"If p then q" is logically equivalent to "not p or q".

The negation of a conditional statement. The negation of "if p then q" is logically equivalent to "p and not q".

$$\neg \left ( p \rightarrow q \right ) \equiv p \wedge \neg q$$

The proposition ¬q → ¬p is called the contrapositive of the proposition p → q. They are logically equivalent. p → q ≡ ¬q → ¬p

Symbolically the contrapositive of

A conditional statement is logically equivalent to its contrapositive.

## Слайд 26

$$\begin{array} { r l } & { ( \alpha \wedge \beta ) \equiv ( \beta \wedge \alpha ) } & { \text {mutativity of $\wedge$} } \\ & { ( \alpha \vee \beta ) \equiv ( \beta \vee \alpha ) } & { \text {mutativity of $\vee$} } \\ & { ( ( \alpha \wedge \beta ) \wedge \gamma ) \equiv ( \alpha \wedge ( \beta \wedge \gamma ) ) } & { \text {associativity of $\wedge$} } \\ & { ( ( \alpha \vee \beta ) \vee \gamma ) \equiv ( \alpha \vee ( \beta \vee \gamma ) ) } & { \text {associativity of $\vee$} } \\ & { \neg ( \neg \alpha ) \equiv \alpha } & { \text {double-negativity elimination} } \\ & { ( \alpha \Rightarrow \beta ) \equiv ( \neg \beta \Rightarrow \neg \alpha ) } & { \text {contraposition} } \\ & { ( \alpha \Rightarrow \beta ) \equiv ( \neg \alpha \vee \beta ) } & { \text {implification elimination} } \\ & { ( \alpha \leftrightarrow \beta ) \equiv ( ( \alpha \Rightarrow \beta ) \wedge ( \beta \Rightarrow \alpha ) ) } & { \text {biconditional elimination} } \\ & { \neg ( \alpha \wedge \beta ) \equiv ( \neg \alpha \vee \neg \beta ) } & { \text {De Morgan} } \\ & { \neg ( \alpha \vee \beta ) \equiv ( \neg \alpha \wedge \neg \beta ) } & { \text {De Morgan} } \\ & { ( \alpha \wedge ( \beta \vee \gamma ) ) \equiv ( ( \alpha \wedge \beta ) \vee ( \alpha \wedge \gamma ) ) } & { \text {distributivity of $\wedge$ over $\vee$} } \\ & { ( \alpha \vee ( \beta \wedge \gamma ) ) \equiv ( ( \alpha \vee \beta ) \wedge ( \alpha \vee \gamma ) ) } & { \text {distributivity of $\vee$ over $\vee$} } \end{array}$$

Figure 7.11 Standard logical equivalences. The symbols α, β, and γ stand for arbitrary sentences of propositional logic.

## Слайд 27

## Propositional logic (Recap)

- Logical constants: true, false
- Propositional symbols: P, Q, S, ... (atomic sentences)
- Wrapping parentheses: ( … )
- Sentences are combined by connectives:
- Literal: atomic sentence or negated atomic sentence

```
- Sentence is combined by connections:
      \___and
      \___implies [impllication / conditional]
      \=>.is equivalent       [biconditional]
      \___not                  [negation]
  - Literal: atomic sentence or negated atom
```

## Слайд 28

## Propositional Logic (Recap)

- A simple language useful for showing key ideas and definitions
- User defines a set of propositional symbols, like P and Q.
- User defines the semantics of each propositional symbol:
- P means "It is hot"
- Q means "It is humid"
- R means "It is raining"
- A sentence (well formed formula) is defined as follows:
- A symbol is a sentence
- If S is a sentence, then ¬S is a sentence
- If S is a sentence, then (S) is a sentence
- If S and T are sentences, then (S ∨ T), (S ∧ T), (S → T), and (S T) are sentences
- A sentence results from a finite number of applications of the above rules

## Слайд 29

## Truth tables

The five logical operators:

| P                     | Q                     | ¬P                    | P∧Q                    | PVQ                  | P ⇒ Q                | P ⇔ Q                 |
|-----------------------|-----------------------|-----------------------|------------------------|----------------------|----------------------|-----------------------|
| False False True True | False True False True | True True False False | False False False True | False True True True | True True False True | True False False True |

## Слайд 30

## Truth Tables through SETs

<!-- image -->

## Слайд 31

## Exercise

Are the statements P →(Q ∨R) and (P→Q)∨(P→R) logically equivalent? Explain your answer.

## Слайд 32

## Exercise

Are the statements P →(Q ∨R)(1) and (P→Q)∨(P→R) (2)logically equivalent? Explain your answer.

Solution 1. Truth Table.

## Solution 2.

Step 1. (Implication Elimination) (1) P →(Q ∨R) ≡ ¬ p ∨ (q ∧ r).

Step 2. (Implication Elimination) (2) (p → q) ∧ (p → r) ≡ (¬ p ∨ q) ∧ (¬ p ∨ r)

Step 3. (Distribution law) (2) (¬ p ∨ q) ∧ (¬ p ∨ r) ≡ ¬p ∨ (q ∧ r) (1)

## Слайд 33

## How can we express "Every person is mortal." with propositional logic?

## Слайд 34

## How can we express "Every person is mortal." with propositional logic?

## WE CAN NOT. WE NEED FIRST-ORDER LOGIC!!!

## Слайд 35

## First-Order Logic

## Слайд 36

## Propositional logic is a weak language

- Hard to identify "individuals" (e.g., Mary, 3)
- Can't directly talk about properties of individuals or relations between individuals (e.g., "Bill is tall")
- Generalizations, patterns, regularities can't easily be represented (e.g., "all triangles have 3 sides")
- First-Order Logic (abbreviated FOL) is expressive enough to concisely represent this kind of information
- FOL adds relations, variables, and quantifiers, e.g.,
- "Every elephant is gray": ∀ x (elephant(x) → gray(x))
- "There is a white alligator": ∃ x (alligator(X) ^ white(X))

## Слайд 37

## First-order logic

- First-order logic (FOL) models the world in terms of
- Objects, which are things with individual identities
- Properties of objects that distinguish them from others
- Relations that hold among sets of objects
- Functions, which are a subset of relations where there is only one "value" for any given "input"
- Examples:
- -Objects: Students, lectures, companies, cars ...
- Relations: Brother-of, bigger-than, outside, part-of, has-color, occurs-after, owns, visits, precedes, ...
- Properties: blue, oval, even, large, ...
- Functions: father-of. best-friend. second-half. more-than ..

## Слайд 38

## FOL Provides

## · Variable symbols

- -E.g., x, y, foo
- Connectives
- -Same as in propositional logic: not (¬), and (∧), or (∨), implies (→), iff ( )

## · Quantifiers

- -Universal ∀x or (Ax)
- -Existential ∃x or (Ex)

## Слайд 39

## Sentences: built from terms and atoms

- quantified sentences adds quantifiers ∀ and ∃
- -∀x loves(x, mother(x))
- -∃x number(x) ∧ greater(x, 100) ∧ prime(x)
- A well-formed formula (wff) is a sentence containing no "free" variables, i.e., all variables are "bound" by either a universal or existential quantifiers

(∀x)P(x,y) has x bound as a universally quantified variable, but y is free

## Слайд 40

## Quantifiers

## · Universal quantification

- -(∀x)P(x) means P holds for all values of x in domain associated with variable
- -E.g., (∀x) dolphin(x) → mammal(x)

## · Existential quantification

- -(∃x)P(x) means P holds for some value of x in domain associated with variable
- -E.g., (∃x) mammal(x) ∧ lays\_eggs(x)
- -This lets us make a statement about some object without naming it

## Слайд 41

<!-- image -->

二

## Quantifier Scope

- FOL sentences have structure, like programs
- In particular, the variables in a sentence have a scope
- For example, suppose we want to say
- -"everyone who is alive loves someone"
- -(∀x) alive(x) → (∃y) loves(x,y)
- Here's how we scope the variables

(∀x) alive(x) → (∃y) loves(x,y)

Scope of x

Scope of y

## Слайд 42

## Quantifier Scope

- Switching order of universal quantifiers does not change the meaning
- (∀x)(∀y)P(x,y) (∀y)(∀x) P(x,y)
- "Dogs hate cats" (i.e., "all dogs hate all cats")
- You can switch order of existential quantifiers
- (∃x)(∃y)P(x,y) (∃y)(∃x) P(x,y)
- "A cat killed a dog"
- Switching order of universal and existential quantifiers does change meaning:
- Everyone likes someone: (∀x)(∃y) likes(x,y)
- Someone is liked by everyone: (∃y)(∀x) likes(x,y)

## Слайд 43

## Connections between ∀ and ∃

- We can relate sentences involving ∀ and ∃ using extensions to De Morgan's laws:

<!-- formula-not-decoded -->

- Examples
1. All dogs don't like cats No dogs like cats
2. Not all dogs dance There is a dog that doesn't dance
3. All dogs sleep There is no dog that doesn't sleep
4. There is a dog that talks Not all dogs can't talk

## Слайд 44

## Exercise I

Translate these English statements into First Order Logic.

1. Every student takes Introduction to AI.
2. There is some course that every student has not taken.

## Слайд 45

## Exercise I - Solutions

Translate these English statements into First Order Logic.

1. Every student takes Introduction to AI.
2. Solution: ∀x, Student(x) =&gt; Takes(x, Introduction to AI)
2. There is some course that every student has not taken.
4. Solution: ∃x, Course (x) [∀y, Student(y)=&gt; ¬Taken (y, x)]

## Слайд 46

## Exercise II

This exercise uses the function Borders(x,y) and Country(x) whose arguments are geographical regions, along with constant symbols for various regions. We give an English sentence and a number of candidate logical expressions. For each of the logical expressions, state whether it (1) correctly expresses the English sentence; or (2) is syntactically valid but does not express the meaning of the English sentence.

## All countries that border Ecuador are in South America.

1. ∀cCountry(c)∧Border(c,Ecuador)∧In(c,SouthAmerica)
2. ∀cCountry(c)∧Border(c,Ecuador)⇒In(c,SouthAmerica)
3. ∀cCountry(c)⇒[Border(c,Ecuador)⇒In(c,SouthAmerica)]

## Слайд 47

## Exercise II - Solution

1. ∀cCountry(c)∧Border(c,Ecuador)∧In(c,SouthAmerica) Answer: Does not correctly express the sentence. It reads: "Every country borders Ecuador and is in South America" 2. ∀cCountry(c)∧Border(c,Ecuador)⇒In(c,SouthAmerica) Answer: Correctly express the English sentence.

```
3.	\cCountry(c)=>[Border(c,Ecuador)=>In(c,SouthAmerica)]	=
\c	~Country(c) V	~Border(c,Ecuador) V	In(c,SouthAmerica)]	=
\c	[~Country(c) V	~Border(c,Ecuador)] V	In(c,SouthAmerica)	=
\c	~([Country(c)	^~Border(c,Ecuador)] V	In(c,SouthAmerica)	=
\cCountry(c)\ABorder(c,Ecuador)=>In(c,SouthAmerica)	-
Answer: Equivalent with 2.

```

## Слайд 48

## Exercise III

- (a) If a person is 18 years or older, then he/she is eligible to vote.
- (b) (b) A person does not graduate from high school if he/she is not at least 18 years old.
- (c) (c) Sarah is a person and she attended the high school graduation ceremony last week.
- (d) (d) If a person completes high school graduation requirements, then he/she is considered to be graduated from high school.
- (e) (e) if a high school student attends the graduation ceremony, then the student has completed high school graduation requirements.

Is Sarah eligible to vote?

## Слайд 49

## Logic Puzzle

1. Adam doesn't live on the top floor
2. Bill doesn't live on the bottom floor
3. Claire doesn't live on either the top or the buttom floor
4. David lives on a higher floor than Bill's
5. Eric doesn't live on a floor adjacent to Claire's
6. Claire doesn't live on a floor adjacent to Bill's

## Слайд 50

<!-- image -->