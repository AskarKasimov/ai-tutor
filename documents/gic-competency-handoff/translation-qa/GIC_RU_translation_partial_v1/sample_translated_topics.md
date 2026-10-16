# AI-01-01 — Подходы к определению искусственного интеллекта

**Вопрос:** Классифицируй три описанные системы по подходам к определению ИИ и объясни пограничный случай.

**Provides:** различать четыре подхода к определению ИИ по поведению и мышлению; классифицировать ИИ-систему по используемому критерию

**Requires:** понимание различия между наблюдаемым поведением и внутренним процессом

## Перевод

[D27 | week1 lecture.pdf | с.13 | core]
Введение
Введение во введение в искусственный интеллект.

[D27 | week1 lecture.pdf | с.15 | core]
Искусственный интеллект
- Выделяют четыре основных подхода к определению ИИ:
  - мышление по образцу человека (thinking humanly);
  - рациональное мышление (thinking rationally);
  - поведение по образцу человека (acting humanly);
  - рациональное поведение (acting rationally).
- Шесть основных областей исследований (по Расселу и Норвигу):
  - обработка естественного языка;
  - представление знаний;
  - автоматизированное рассуждение;
  - машинное обучение;
  - компьютерное зрение;
  - робототехника.

[D27 | week1 lecture.pdf | с.16 | core]
Дальнейшие направления ИИ
- Вычислительное творчество.
- Биомиметика (подражание природным механизмам).

## Оригинал

[D27 | week1 lecture.pdf | с.13 | core]
Introduction
The Introduction to the Introduction to AI

[D27 | week1 lecture.pdf | с.15 | core]
Artificial Intelligence
- Breaks down into four major definitions classes:
- Thinking Humanly
- Thinking Rationally
- Acting Humanly
- Acting Rationally
- Six major areas of concern (Russel & Norvig)
- Natural Language Processing
- Knowledge Representation
- Automated Reasoning
- Machine Learning
- Computer Vision
- Robotics

[D27 | week1 lecture.pdf | с.16 | core]
AI Onwards
- Computational Creativity
- Biomimicry

---

# AI-02-02 — Рациональность и ограниченность знаний агента

**Вопрос:** Выбери рациональное действие при неполной информации и объясни отличие рациональности от всеведения.

**Provides:** обосновывать рациональность по доступной информации и цели; отличать рациональность от всеведения и гарантированного успеха

**Requires:** агент, наблюдения и действия; критерий успешности

## Перевод

[D19 | Week 3 Lecture.pdf | с.6 | core]
Рациональность
- То, что считается рациональным в конкретный момент, зависит от:
  - показателя успешности (performance measure), определяющего результат;
  - априорных знаний агента об окружающей среде;
  - действий, доступных агенту;
  - последовательности восприятий, полученных агентом к данному моменту.
- Определение: для любой возможной последовательности восприятий рациональный агент должен выбирать действие, которое, с учётом этих восприятий и встроенных знаний, предположительно максимизирует показатель успешности.

[D19 | Week 3 Lecture.pdf | с.7 | core]
Агенты рациональны, но не обязательно всеведущи
- Шут опрокидывает королевский бокал вина.
- «Вот глупец!»
- Возможно, шут выполнял колесо и неудачно приземлился?
- А возможно, шут заметил, что вино отравили, а король этого не увидел?
- Прежде чем действовать, агентам нужно собирать информацию.
- «Зачем ты разлил моё вино, шут?!»
- Агенты могут воспринимать ситуацию по-разному.
- Агенты обладают определённой автономией: король не контролирует каждое действие шута.

## Оригинал

[D19 | Week 3 Lecture.pdf | с.6 | core]
Rationality
- What is rational at any given time depends upon:
- The performance measure defines success
- The agent’s prior knowledge of the environment
- The actions which an agent can perform
- The agent’s percept sequence to date
- Definition:
- For Each possible percept sequence, a rational agent should select an action
that is expected to maximize its performance measure, given the evidence
provided by the percept sequence and whatever built-in knowledge the agent
has

[D19 | Week 3 Lecture.pdf | с.7 | core]
Agents are Rational,
Not Necessarily Omniscient
- A jester knocks over the king’s wine
- “What an idiot!”
- Did the jester make a cartwheel and miss the landing?
- Did the jester see it being poisoned and the king did not?
- Agents need to gather information before making a decision of an action
- “Why did you spill my wine jester?!”
- Note that agents may perceive differently from each other
- Agents have a degree of autonomy
- The King does not control every action of the jester

---

# AI-04-07 — Допустимость эвристики

**Вопрос:** Определи на конкретных оценках, допустима ли эвристика, и покажи контрпример.

**Provides:** проверять h(n)≤h*(n) для допустимости; объяснять связь недооценки и оптимальности поиска

**Requires:** эвристическая оценка; минимальная стоимость до цели

## Перевод

[D03 | Lecture 5.pdf | с.16 | core]
Допустимость эвристики
- В задачах поиска пути эвристика может быть допустимой или недопустимой.
- Допустимость означает, что эвристика не переоценивает истинную стоимость пути до цели.
- Оценка h должна быть не больше фактической минимальной стоимости достижения цели.
- Допустимая эвристика, следовательно, оптимистична.
- Не каждая эвристика обязана быть допустимой. Однако при использовании недопустимой эвристики оптимальность найденного решения не гарантируется.

[D22 | Week 6 Lecture.pdf | с.16 | shared]
Допустимость и согласованность эвристик
- Эвристика допустима, если не переоценивает минимальную стоимость пути до цели.
- Не всякая эвристика обязана быть допустимой, но недопустимая не обеспечивает гарантию оптимального решения.
- Согласованной (монотонной) называется эвристика, для которой оценка стоимости от текущей вершины до цели не больше стоимости перехода к соседу плюс эвристическая оценка от этого соседа до цели.
- Стандартная запись: h(x) ≤ c(x, y) + h(y) для соседних x, y. [Формула на слайде распознана с искажениями; математическое условие восстановлено по определению.]
- Каждая согласованная эвристика допустима, если h(цель) = 0; обратное в общем случае неверно.

## Оригинал

[D03 | Lecture 5.pdf | с.16 | core]
Heuristics Admissibility
- In path-finding problems heuristics may be admissible or inadmissible
- Admissibility means that heuristics is not overestimating the solution
- Needs to be lower than or equal to the actual cost of reaching the goal
- An admissible heuristic is therefore optimistic
- Not each heuristic is obliged to be admissible. However, inadmissible
heuristic does not guarantee the optimal solution

[D22 | Week 6 Lecture.pdf | с.16 | shared]
Heuristics Admissibility and Consistency
- In path-finding problems heuristics may be admissible or inadmissible
- Admissibility means that heuristics is not overestimating the solution
- Needs to be lower than or equal to the actual cost of reaching the goal
- Not each heuristic is obliged to be admissible. However, inadmissible heuristic
does not guarantee the optimal solution
- In path-finding problems a heuristic is considered to be consistent (or
monotone) if the estimated cost from one node to the successor node, added
to the estimated cost from the successor node to the goal is less than or equal
to the estimated cost from the current node to the goal state
- 𝐸𝑓(𝑥! →𝑥!"#) + 𝐸𝑓(𝑥!"# →𝑥$%&') ≤𝐸𝑓(𝑥! →𝑥$%&') →consistent heuristics
- All consistent heuristics are admissible heuristics, however, all admissible
heuristics are not necessarily consistent heuristics

---

# AI-05-08 — Параллелизация Monte Carlo Tree Search

**Вопрос:** Для ускорения MCTS сравни leaf, root и tree parallelization, выбери одну схему для заданных условий и укажи проблему синхронизации.

**Provides:** сравнивать три схемы параллельного MCTS; объяснять конфликты одновременных обновлений дерева

**Requires:** MCTS-дерево, rollout и накопленная статистика; параллельные вычисления и общий доступ к данным

## Перевод

[D25 | Week 7 Lecture.pdf | с.36 | core]
Параллелизация MCTS
**Параллелизация на уровне листьев (leaf parallelization).** Одновременно выполняется несколько симуляций продолжения игры (playouts), начиная с одного листа игрового дерева.

**Параллелизация на уровне корня (root parallelization).** Независимые игровые деревья строятся параллельно; ход выбирается на основании ветвей у корня всех полученных деревьев.

**Параллелизация дерева (tree parallelization).** Несколько вычислителей параллельно строят одно общее дерево. От одновременной записи в данные защищаются с помощью одного глобального мьютекса, нескольких мьютексов или неблокирующей синхронизации.

## Оригинал

[D25 | Week 7 Lecture.pdf | с.36 | core]
Parallelization
Leaf parallelization
Parallel execution of many playouts from one leaf of the game tree
Root parallelization
Building independent game trees in parallel and making the move based on the
root-level branches of all these trees
Tree parallelization
Parallel building of the same game tree, protecting data from simultaneous
writes either with one, global mutex, with more mutexes, or with non-blocking
synchronization

---

# AI-10-09 — Проектирование промпта для LLM

**Вопрос:** Улучши неоднозначный запрос, задав цель, контекст, роль и формат, и обоснуй выбор.

**Provides:** формулировать конкретный prompt с целью, контекстом и форматом; объяснять, как неоднозначность ухудшает ответ

**Requires:** представление о текстовом запросе к LLM

## Перевод

[D01 | 14 Tutorial.pdf | с.7 | core]
Промптинг: взаимодействие с большими языковыми моделями
- Промпт — инструкция, которую вы даёте модели.
- Хороший промпт задаёт роль, контекст и желаемый формат результата.
- Качество промпта сильно влияет на качество ответа.
- Промпт можно рассматривать как программирование на естественном языке.

[D01 | 14 Tutorial.pdf | с.8 | core]
Пример улучшения промпта
- Плохой вариант: «Расскажи мне о конференции».
- Более удачный вариант: «Ты — помощник по конференции ICSE 2026. Пятью пунктами опиши мероприятие: дату, место проведения, основные темы, целевую аудиторию и крайний срок регистрации».
- Задание: предложите, как ещё улучшить этот промпт — например, уточнить тональность, ограничить длину ответа и т. д.

## Оригинал

[D01 | 14 Tutorial.pdf | с.7 | core]
Prompting: Talking to LLMs
- A prompt is the instruction you give the model
- Good prompts specify a role, context, and desired format
- Prompt quality strongly affects output quality
- Think of prompts as programming with natural language

[D01 | 14 Tutorial.pdf | с.8 | core]
Example: Prompt Improvement
- Bad: "Tell me about the conference."
- Better: "You are an assistant for the 'ICSE 2026' conference.
In 5 bullet points, describe the event (date, location, main
topics, target audience, registration deadline)."
- Activity: Propose ways to improve this prompt further
(tone, length limits, etc.).

---

